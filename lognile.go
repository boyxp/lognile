package lognile

import "os"
import "log"
import "time"
import "sync"
import "syscall"
import "os/signal"
import "path/filepath"
import "encoding/json"
import "gopkg.in/yaml.v3"
import "github.com/fsnotify/fsnotify"

type Lognile struct {
	db string
	node sync.Map
	offset sync.Map
	registrar sync.Map
	pattern map[string][]string
	log chan map[string]string
	watcher *fsnotify.Watcher
	callback func(log map[string]string)
	exit bool
}

func (L *Lognile) Init(cfg string, callback func(log map[string]string)) {
	log.Println("启动...")

	config := L.config(cfg)

	log.Println("解析配置文件:", cfg)

	if v, ok := config["db"];ok {
		L.db = v.(string)
	} else {
		L.db = "lognile.db"
	}

	log.Println("读取进度数据:", L.db)

	L.load(L.db)

	pattern, ok := config["pattern"]
	if !ok {
		panic("没有配置日志监听路径")	
	}
	L.parse(pattern)

	L.callback = callback
	L.log      = make(chan map[string]string, 1000)




	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("文件夹监听初始化失败", err)
	}
	L.watcher = watcher
	defer watcher.Close()

	log.Println("启动文件夹监听")

	for dir, _ := range L.pattern {
		log.Println("监听日志文件夹:", dir)

		L.add(dir)
		L.watcher.Add(dir)
	}

	log.Println("启动监听事件消费进程")
    go L.listen(watcher)

    log.Println("启动日志实时回调")
	go func() {
		for{
			select {
				case v := <-L.log:
						L.callback(v)
				default :
			}
		}
	}()

	log.Println("监听进程退出信号")

	L.signal()

	log.Println("启动成功")

	<-make(chan struct{})
}

func (L *Lognile) config(cfg string) map[string]any {
	content, err := os.ReadFile(cfg)
    if err != nil {
        log.Fatal("配置文件读取失败:", err)
    }

    data := make(map[string]any)
    err   = yaml.Unmarshal(content, &data)
    if err != nil {
        log.Fatal("配置文件解析失败:", err)
    }

    return data
}

func (L *Lognile) parse(pattern any) {
	L.pattern = map[string][]string{}

	for _, p := range pattern.([]any) {
		abs, err := filepath.Abs(p.(string))
		if err!=nil {
			log.Println("文件路径转绝对路径失败,file:", p.(string), "error:", err)
			continue
		}

		dir  := filepath.Dir(abs)
		base := filepath.Base(abs)

		if _, ok := L.pattern[dir];ok {
			L.pattern[dir] = append(L.pattern[dir], base)
		} else {
			L.pattern[dir] = []string{base}
		}
	}
}

func (L *Lognile) load(db string) {
	_, err := os.Stat(db)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Fatal("db文件加载失败:", err)
	}

	content, err := os.ReadFile(db)
	if err != nil {
		log.Fatal("db文件读取失败:", err)
	}

	var offset map[uint64]int64
	err = json.Unmarshal(content, &offset)
	if err != nil {
		log.Fatal("db文件解析失败:", err)
	}

	for _node,_offset := range offset {
		L.offset.Store(_node, _offset)
	}
}

func (L *Lognile) save(db string) {
	offset := map[uint64]int64{}

	L.registrar.Range(func(key any, value any) bool {
		_node, _ := key.(uint64)
		_offset  := value.(*Reader).Offset()
        offset[_node] = _offset
        return true
    })

	content, err := json.Marshal(offset)
	if err != nil {
		log.Println("请手动保存进度数据：", offset)
		log.Fatal("进度数据编码失败:", err)
	}

	if err := os.WriteFile(db, []byte(content), 0666); err != nil {
		log.Println("请手动保存进度数据：", content)
		log.Fatal("读取进度数据存储失败:", err)
	}
}

func (L *Lognile) inode(file string) uint64 {
	value, ok := L.node.Load(file)
	if ok {
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
	for _, p := range L.pattern[dir] {
		list, err := filepath.Glob(dir+"/"+p)
		if err!=nil {
			log.Println("文件夹文件匹配失败:", err)
			continue
		}

		for _, file := range list {
			var offset int64
			node   := L.inode(file)
			v, ok := L.offset.Load(node)
			if ok {
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
					reader, ok := L.registrar.Load(node)
					if !ok {
						L.create(event.Name)
						log.Println("发现新日志文件:", event.Name)
					} else {
						_reader := reader.(*Reader)
						_name   := _reader.Name()
						_reader.Rename(event.Name)
						log.Println("文件改名:", _name, "->", event.Name)
					}
				}

                //if event.Has(fsnotify.Rename) {
                //}

				if event.Has(fsnotify.Write) {
					node  := L.inode(event.Name)
					reader, ok := L.registrar.Load(node)
					if ok {
						go reader.(*Reader).Read(true, L.log)
					}
				}

				if event.Has(fsnotify.Remove) {
					log.Println("日志文件被删除:", event.Name)
					node  := L.inode(event.Name)
					reader, ok := L.registrar.Load(node)
					if ok {
						reader.(*Reader).Close()
						L.registrar.Delete(node)
					}
				}

			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
		}
	}
}

func (L *Lognile) create(file string) {
	abs, err := filepath.Abs(file)
	if err!=nil {
		log.Println("文件获取绝对路径失败,file:", file, "error:", err)
		return
	}

	dir  := filepath.Dir(abs)
	base := filepath.Base(abs)

	match := false
	for _, p := range L.pattern[dir] {
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
	L.exit = true

	log.Println("等待读取进程退出...3s")
	time.Sleep(time.Second)
	log.Println("等待读取进程退出...2s")
	time.Sleep(time.Second)
	log.Println("等待读取进程退出...1s")
	time.Sleep(time.Second)

	log.Println("关闭文件句柄...")
	L.registrar.Range(func(node any, reader any) bool {
		reader.(*Reader).Close()
        return true
    })
	log.Println("关闭文件句柄成功")

	log.Println("保存日志进度...")
	L.save(L.db)
	log.Println("保存日志进度成功")

	log.Println("进程退出成功")

	os.Exit(0)
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
