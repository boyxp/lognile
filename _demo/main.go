package main

import "os"
import "log"
import "time"
import "strconv"
import "github.com/boyxp/lognile"

func main() {
	go Write("./_log/php-fpm-1.log", "php-query")
	go Write("./_log/nginx-1.log", "nginx-query")

	L := lognile.Lognile{}
	L.Init("config.yaml", Print)
}

func Print(row map[string]string) {
	log.Println("日志：", row)
}

func Write(file string, content string) {
	fp, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer fp.Close()

	for i:=0;i<10;i++ {
		if _, err := fp.WriteString(content+"\t"+strconv.Itoa(i)+"\n"); err != nil {
			panic(err)
		}
		time.Sleep(time.Duration(1) * time.Second)
	}
}
