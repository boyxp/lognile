package main

import "os"
import "log"
import "gopkg.in/yaml.v3"

func main() {
	L := Lognile{}
	L.Init()

	log.Println("ok")
}

type Lognile struct {}

func (L *Lognile) Init() {
	log.Println(L.config("config.yaml"))
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
