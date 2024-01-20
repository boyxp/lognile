package main

import "os"
import "io"
import "log"
import "bufio"
import "syscall"
import "os/signal"
import "path/filepath"
import "encoding/json"
import "gopkg.in/yaml.v3"
import "github.com/fsnotify/fsnotify"

func main() {
	L := Lognile{}
	L.Init()

	log.Println("ok")
}

type Lognile struct {
	offset map[uint64]int64
	db string
	pattern map[string][]string
	log chan map[string]string
	fp map[uint64]*os.File
	node map[string]uint64
	watcher *fsnotify.Watcher
}

func (L *Lognile) Init() {
	config := L.config("config.yaml")

	if v, ok := config["db"];ok {
		L.db = v.(string)
	} else {
		L.db = "lognile.db"
	}

	L.offset = L.load(L.db)

	pattern, ok := config["pattern"]
	if !ok {
		panic("没有配置日志路径")	
	}
	L.parse(pattern)

	L.log   = make(chan map[string]string, 1000)
	L.fp    = map[uint64]*os.File{}
	L.node = map[string]uint64{}

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
    }
    L.watcher = watcher
    defer watcher.Close()


	for dir, _ := range L.pattern {
		L.add(dir)
		L.watcher.Add(dir)
	}

    go L.listen(watcher)

	//log.Println(L.inode("log.db"))

	go func() {
		for{
        	select {
        		case v := <-L.log:
            		log.Println(v)
            	default :
    		}
    	}
	}()

	L.exit()

	<-make(chan struct{})
}

func (L *Lognile) config(cfg string) map[string]any {
	content, err := os.ReadFile(cfg)
    if err != nil {
        log.Fatal(err)
    }

    data := make(map[string]any)
    err   = yaml.Unmarshal(content, &data)
    if err != nil {
        log.Fatalf("error: %v", err)
    }

    return data
}

func (L *Lognile) parse(pattern any) {
	L.pattern = map[string][]string{}

	for _, p := range pattern.([]any) {
		abs, err := filepath.Abs(p.(string))
		if err!=nil {
			log.Fatal(err)
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

func (L *Lognile) load(db string) map[uint64]int64 {
	_, err := os.Stat(db)
	if err != nil {
		if os.IsNotExist(err) {
			return map[uint64]int64{}
		}
		log.Fatal(err)
	}

	content, err := os.ReadFile(db)
	if err != nil {
		log.Fatal(err)
	}

	var offset map[uint64]int64
	err = json.Unmarshal(content, &offset)
	if err != nil {
		log.Fatal(err)
	}

	return offset
}

func (L *Lognile) save(db string) {
	content, err := json.Marshal(L.offset)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(db, []byte(content), 0666); err != nil {
		log.Fatal(err)
	}
}

func (L *Lognile) inode(file string) uint64 {
	_node, ok := L.node[file]
	if ok {
		return _node
	}

	var stat syscall.Stat_t
	if err := syscall.Stat(file, &stat); err != nil {
		panic(err)
	}

	L.node[file] = stat.Ino

	log.Println("node", file, stat.Ino)

	return stat.Ino
}

func (L *Lognile) add(dir string) {
	files := []string{}

	for _, p := range L.pattern[dir] {
		list, err := filepath.Glob(dir+"/"+p)
		if err!=nil {
			log.Println(err)
			continue
		}

		for _, file := range list {
			files = append(files, file)
			L.read(file)
		}
	}

	log.Println(files)
}

func (L *Lognile) read(file string) {
	fp := L.open(file)

	reader := bufio.NewReader(fp)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			if len(line)>1 {
				L.log <- map[string]string{"file-end":file, "log":line[:len(line)-1]}
			}
			break
		}
		if err != nil {
			panic(err)
		}
		//log.Println()
		if len(line)>1 {
			L.log <- map[string]string{"file-mid":file, "log":line[:len(line)-1]}
		}
	}

	offset, _ := fp.Seek(0, 1)

	_node := L.inode(file)
	L.offset[_node] = offset

	log.Println("offset", file, offset)
}

func (L *Lognile) open(file string) *os.File {
	node  := L.inode(file)
	fp,ok := L.fp[node]
	if !ok {
		_fp, err := os.Open(file)
		if err != nil {
			panic(err)
		}

		offset, ok := L.offset[node]
		if ok {
			_fp.Seek(offset, 0)
			log.Println("重置指针位置", offset)
		}

		fp = _fp
		L.fp[node] = _fp
	}

	return fp
}

func (L *Lognile) listen(watcher *fsnotify.Watcher)  {
        log.Println("start")
        for {
            select {
            case event, ok := <-watcher.Events:
                if !ok {
                    return
                }
                log.Println("event:", event.Op, event.Name)

                if event.Has(fsnotify.Create)  {
                    fid := L.inode(event.Name)
                    _, ok := L.offset[fid]
                    if ok {
                        log.Println("旧文件，采用原进度")
                    } else {
                        L.create(event.Name)
                        log.Println("新文件,添加新节点")
                    }
                }

                if event.Has(fsnotify.Rename) {
                    log.Println("被改名或移动")
                }

                if event.Has(fsnotify.Write) {
                    log.Println("文件写入")
                    L.read(event.Name)
                }

                if event.Has(fsnotify.Remove) {
                    log.Println("被删除")
                    L.delete(event.Name)
                }

            case err, ok := <-watcher.Errors:
                if !ok {
                    return
                }
                log.Println("error:", err)
            }
        }
        log.Println("exit")
}

func (L *Lognile) create(file string) {
	abs, err := filepath.Abs(file)
	if err!=nil {
		log.Fatal(err)
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
		log.Println("不匹配的文件", file)
		return
	}

	log.Println("新文件读取", file)

	L.read(file)
}

func (L *Lognile) delete(file string) {
	log.Println("被删除", file)

	fid := L.inode(file)
    _, ok1 := L.offset[fid]
    if ok1 {
    	log.Println("删除进度", file)
    	delete(L.offset, fid)
    }

    fp, ok2 := L.fp[fid]
    if ok2 {
    	log.Println("关闭文件", file)
    	fp.Close()
    }
}

func (L *Lognile) exit() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, 
                         syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
				case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
					log.Println("Program Exit...", s)
					L.save(L.db)
					os.Exit(0)
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
