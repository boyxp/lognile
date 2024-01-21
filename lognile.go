package main

import "os"
import "io"
import "log"
import "time"
import "sync"
import "bufio"
import "syscall"
import "os/signal"
import "path/filepath"
import "encoding/json"
import "gopkg.in/yaml.v3"
import "github.com/fsnotify/fsnotify"

func main() {
	L := Lognile{}
	L.Init("config.yaml", Print)

	log.Println("ok")
}

func Print(row map[string]string) {
	log.Println("日志：", row)
}






type Lognile struct {
	db string
	node sync.Map
	offset sync.Map
	handler sync.Map
	pattern map[string][]string
	log chan map[string]string
	watcher *fsnotify.Watcher
	callback func(log map[string]string)
	exit bool
}

func (L *Lognile) Init(cfg string, callback func(log map[string]string)) {
	log.Println("启动")

	config := L.config(cfg)

	log.Println("解析配置文件成功:", cfg)

	if v, ok := config["db"];ok {
		L.db = v.(string)
	} else {
		L.db = "lognile.db"
	}

	log.Println("读取进度数据库文件为:", L.db)

	L.load(L.db)

	log.Println("加载数据库进度文件成功")

	pattern, ok := config["pattern"]
	if !ok {
		panic("没有配置日志监听路径")	
	}
	L.parse(pattern)

	L.callback = callback
	L.log      = make(chan map[string]string, 1000)


	log.Println("启动文件夹监听")

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal("文件夹监听初始化失败", err)
    }
    L.watcher = watcher
    defer watcher.Close()

    log.Println("文件夹监听进程启动成功")

	for dir, _ := range L.pattern {
		log.Println("添加日志监控文件夹:", dir)

		L.add(dir)
		L.watcher.Add(dir)
	}

	log.Println("启动监听事件消费")
    go L.listen(watcher)

    log.Println("启动日志实时输出")
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

	L.offset.Range(func(key any, value any) bool {
		_node, _     := key.(uint64)
		_offset, _   := value.(int64)
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
			go L.read(file, false)
		}
	}
}

func (L *Lognile) read(file string, wait bool) {
	handler := L.open(file)
	if handler.Lock()==false {
		return
	}

	log.Println("加锁", file)

	retry  := 0
	fp     := handler.Pointer()
	reader := bufio.NewReader(fp)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			if len(line)>1 {
				L.log <- map[string]string{"file":file, "log":line[:len(line)-1]}
			}

			if wait==false || retry>2 || L.exit==true {
				break
			}

			time.Sleep(time.Duration(1) * time.Second)

			log.Println("休息重试", file, retry)

			retry++

			continue
		}

		if err != nil {
			log.Println("文件日志读取失败,file:",file, "error:", err)
			break
		}

		if len(line)>1 {
			L.log <- map[string]string{"file":file, "log":line[:len(line)-1]}
		}

		if L.exit==true {
			break
		}

		retry = 0
	}

	_node          := L.inode(file)
	offset, _      := fp.Seek(0, 1)
	L.offset.Store(_node, offset)

	handler.Unlock()

	log.Println("解锁", file)
}

func (L *Lognile) open(file string) *Handler {
	node        := L.inode(file)
	handler,ok  := L.handler.Load(node)
	if !ok {
		fp, err := os.Open(file)
		if err != nil {
			log.Fatal("日志文件打开失败,file:", file, "error:", err)
		}

		offset, ok := L.offset.Load(node)
		if ok {
			_offset, _ := offset.(int64)
			fp.Seek(_offset, 0)
		}

		handler = &Handler{pointer:fp}

		L.handler.Store(node, handler)
	}

	return handler.(*Handler)
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
					_, ok := L.offset.Load(node)
					if !ok {
						L.create(event.Name)
						log.Println("发现新日志文件:", event.Name)
					}
				}

                //if event.Has(fsnotify.Rename) {
                //}

				if event.Has(fsnotify.Write) {
					go L.read(event.Name, true)
				}

				if event.Has(fsnotify.Remove) {
					log.Println("日志文件被删除:", event.Name)
					L.delete(event.Name)
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

	go L.read(file, false)
}

func (L *Lognile) delete(file string) {
	node   := L.inode(file)
    _, ok1 := L.offset.Load(node)
    if ok1 {
    	L.offset.Delete(node)
    }

    handler, ok2 := L.handler.Load(node)
    if ok2 {
    	handler.(*Handler).Pointer().Close()
    }
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
	L.handler.Range(func(node any, handler any) bool {
		handler.(*Handler).Pointer().Close()
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


type Handler struct {
	pointer *os.File
	mu sync.Mutex
}

func (H *Handler) Lock() bool {
	return H.mu.TryLock()
}

func (H *Handler) Unlock() {
	H.mu.Unlock()
}

func (H *Handler) Pointer() *os.File {
	return H.pointer
}
