package main

import "os"
import "io"
import "log"
import "bufio"
import "syscall"
import "path/filepath"
import "encoding/json"
import "gopkg.in/yaml.v3"

func main() {
	L := Lognile{}
	L.Init()

	log.Println("ok")
}

type Lognile struct {
	offset map[string]int64
	db string
	pattern map[string][]string
	log chan map[string]string
	fp map[uint64]*os.File
	node map[string]uint64
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



	for dir, _ := range L.pattern {
		L.add(dir)
	}

	log.Println(L.inode("log.db"))

	go func() {
		for{
        	select {
        		case v := <-L.log:
            		log.Println(v)
            	default :
    		}
    	}
	}()

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

func (L *Lognile) load(db string) map[string]int64 {
	_, err := os.Stat(db)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]int64{}
		}
		log.Fatal(err)
	}

	content, err := os.ReadFile(db)
	if err != nil {
		log.Fatal(err)
	}

	var offset map[string]int64
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
			//log.Println(line)
			L.log <- map[string]string{"file":file, "log":line}
			break
		}
		if err != nil {
			panic(err)
		}
		//log.Println(line[:len(line)-1])
		L.log <- map[string]string{"file":file, "log":line}
	}
}

func (L *Lognile) open(file string) *os.File {
	node  := L.inode(file)
	fp,ok := L.fp[node]
	if !ok {
		_fp, err := os.Open(file)
		if err != nil {
			panic(err)
		}

		fp = _fp
		L.fp[node] = _fp
	}

	return fp
}