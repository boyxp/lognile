package main

import "os"
import "log"
import "encoding/json"
import "gopkg.in/yaml.v3"

func main() {
	L := Lognile{}
	L.Init()

	log.Println("ok")
}

type Lognile struct {
	offset map[string]int64
}

func (L *Lognile) Init() {
	//log.Println(L.config("config.yaml"))
	L.offset = L.load("log.db")
	log.Println(L.offset)
	L.offset["a"] = 123
	L.save("log.db")
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
