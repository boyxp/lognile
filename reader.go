package lognile

import "os"
import "io"
import "fmt"
import "log"
import "time"
import "sync"
import "bufio"

type Reader struct {
	close bool
	file string
	offset int64
	mu sync.Mutex
	pointer *os.File
}

func (R *Reader) Read(wait bool, queue chan map[string]string) {
	if R.mu.TryLock()==false {
		return
	}

	_open := R.open()
	if _open==false {
		R.mu.Unlock()
		return
	}

	retry  := 1
	reader := bufio.NewReader(R.pointer)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			if len(line)>1 {
				queue <- map[string]string{"file":R.file, "log":line[:len(line)-1]}
			}

			if wait==false || retry>3 || R.close==true {
				break
			}

			log.Println("等待新记录", R.file, retry, "秒")

			time.Sleep(time.Duration(1) * time.Second)

			retry++

			continue
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "文件日志读取失败,file:"+R.file+",error:"+err.Error())
			break
		}

		if len(line)>1 {
			queue <- map[string]string{"file":R.file, "log":line[:len(line)-1]}
		}

		if R.close==true {
			break
		}

		retry = 0
	}

	offset, _ := R.pointer.Seek(0, 1)
	R.offset   = offset

	R.mu.Unlock()
}

func (R *Reader) open() bool {
	if R.pointer!=nil {
		return true
	}

	fp, err := os.Open(R.file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志文件打开失败,file:"+R.file+",error:"+err.Error())
		return false
	}

	fp.Seek(R.offset, 0)

	R.pointer = fp

	return true
}

func (R *Reader) Close() {
	R.close = true
	if R.pointer!=nil {
		R.pointer.Close()
	}
}

func (R *Reader) Offset() int64 {
	return R.offset
}

func (R *Reader) Name() string {
	return R.file
}

func (R *Reader) Rename(filename string) {
	R.file = filename
}
