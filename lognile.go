package lognile

import "os"
import "log"
import "fmt"
import "time"
import "sync"
import "syscall"
import "os/signal"
import "path/filepath"
import "github.com/fsnotify/fsnotify"

func Init(cfg string, callback func(log map[string]string)) {
	(&Lognile{}).Init(cfg, callback)
}

type Lognile struct {
	db string
	node sync.Map
	offset sync.Map
	registrar sync.Map
	patterns map[string][]string
	log chan map[string]string
	watcher *fsnotify.Watcher
	callback func(log map[string]string)
}

func (L *Lognile) Init(cfg string, callback func(log map[string]string)) {
	log.Println("启动...")

	config    := (&Config{}).Init(cfg)
	L.db       = config.Db()
	L.patterns = config.Pattern()
	log.Println("解析配置文件:", cfg)

	L.offset = (&Offset{L.db}).Load()
	log.Println("读取进度数据:", L.db)

	L.callback = callback
	L.log      = make(chan map[string]string, 1000)


	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("文件夹监听初始化失败", err)
	}
	L.watcher = watcher
	defer watcher.Close()

	log.Println("启动文件夹监听")

	for dir, _ := range L.patterns {
		log.Println("监听日志文件夹:", dir)

		L.add(dir)
		L.watcher.Add(dir)
	}

	log.Println("启动监听事件消费进程")
    go L.listen(watcher)

    log.Println("启动日志实时回调")
	go func() {
		for{
			L.callback(<-L.log)
		}
	}()

	log.Println("启动定时保存进度")
	go func() {
		for{
			time.Sleep(time.Duration(60) * time.Second)
			L.save(false)
			log.Println("自动保存")
		}
	}()

	log.Println("监听进程退出信号")

	L.signal()

	log.Println("启动成功")

	<-make(chan int)
}


func (L *Lognile) inode(file string) uint64 {
	if value, ok := L.node.Load(file);ok {
		node, _ := value.(uint64)
		return node
	}

	var stat syscall.Stat_t
	if err := syscall.Stat(file, &stat); err != nil {
		log.Fatal("获取文件inode失败,file:", file, "error:", err)
	}

	L.node.Store(file, stat.Ino)

	return stat.Ino
}

func (L *Lognile) add(dir string) {
	for _, p := range L.patterns[dir] {
		list, err := filepath.Glob(dir+"/"+p)
		if err!=nil {
			fmt.Fprintf(os.Stderr, "文件夹文件匹配失败:"+err.Error())
			continue
		}

		for _, file := range list {
			var offset int64
			node   := L.inode(file)
			if v, ok := L.offset.Load(node);ok {
				offset, _ = v.(int64)
			} else {
				offset = 0
			}

			reader := &Reader{file:file,offset:offset}
			go reader.Read(false, L.log)
			L.registrar.Store(node, reader)
		}
	}
}

func (L *Lognile) listen(watcher *fsnotify.Watcher)  {
	for {
		select {
			case event, ok := <-watcher.Events:
							if !ok {
								return
							}

				if event.Has(fsnotify.Create)  {
					node  := L.inode(event.Name)
					if reader, ok := L.registrar.Load(node);ok {
						_reader := reader.(*Reader)
						_name   := _reader.Name()
						_reader.Rename(event.Name)
						log.Println("文件改名,监听新文件名:", _name, "->", event.Name)
					} else {
						L.create(event.Name)
						log.Println("发现新文件,开始监听:", event.Name)
					}
				}

                if event.Has(fsnotify.Rename) {
                	log.Println("文件被移动,句柄关闭:", event.Name)
                	node  := L.inode(event.Name)
					if reader, ok := L.registrar.Load(node);ok {
						reader.(*Reader).Close()
					}
                }

				if event.Has(fsnotify.Write) {
					node  := L.inode(event.Name)
					if reader, ok := L.registrar.Load(node);ok {
						go reader.(*Reader).Read(true, L.log)
					}
				}

				if event.Has(fsnotify.Remove) {
					log.Println("文件被删除,句柄关闭,删除采集器:", event.Name)
					node  := L.inode(event.Name)
					if reader, ok := L.registrar.Load(node);ok {
						reader.(*Reader).Close()
						L.registrar.Delete(node)
					}
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Fatal(err)
		}
	}
}

func (L *Lognile) create(file string) {
	abs, err := filepath.Abs(file)
	if err!=nil {
		fmt.Fprintf(os.Stderr, "文件路径转绝对路径失败,file:"+file+",error:"+err.Error())
		return
	}

	dir  := filepath.Dir(abs)
	base := filepath.Base(abs)

	match := false
	for _, p := range L.patterns[dir] {
		_match, err := filepath.Match(p, base)
		if err!=nil {
			panic(err)
		}

		if _match {
			match = true
		}
	}

	if match==false {
		log.Println("不匹配的日志文件:", file)
		return
	}

	node   := L.inode(file)
	reader := &Reader{file:file}
	go reader.Read(false, L.log)
	L.registrar.Store(node, reader)
}

func (L *Lognile) Exit() {
	log.Println("等待读取进程退出...3s")
	time.Sleep(time.Second)
	log.Println("等待读取进程退出...2s")
	time.Sleep(time.Second)
	log.Println("等待读取进程退出...1s")
	time.Sleep(time.Second)

	log.Println("保存进度，关闭文件句柄...")
	L.save(true)
	log.Println("保存进度，关闭文件句柄成功")

	log.Println("进程退出成功")

	os.Exit(0)
}

func (L *Lognile) save(close bool) {
	offset := map[uint64]int64{}
	L.registrar.Range(func(node any, reader any) bool {
		_reader := reader.(*Reader)
		if close==true {
			_reader.Close()
		}
		_node, _ := node.(uint64)
		_offset  := _reader.Offset()
        offset[_node] = _offset
        return true
    })
    (&Offset{L.db}).Save(offset)
}

func (L *Lognile) signal() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, 
                         syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
				case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
					L.Exit()
				case syscall.SIGUSR1:
					log.Println("usr1 signal", s)
				case syscall.SIGUSR2:
					log.Println("usr2 signal", s)
				default:
					log.Println("other signal", s)
			}
		}
	}()
}
