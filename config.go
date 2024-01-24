package lognile

import "os"
import "fmt"
import "log"
import "path/filepath"
import "gopkg.in/yaml.v3"

type Config struct {
	db string
	patterns map[string][]string
}

func (C *Config) Init(cfg string) *Config {
	content, err := os.ReadFile(cfg)
    if err != nil {
        log.Fatal("配置文件读取失败:", err)
    }

    data := make(map[string]any)
    err   = yaml.Unmarshal(content, &data)
    if err != nil {
        log.Fatal("配置文件解析失败:", err)
    }

	if v, ok := data["db"];ok {
		C.db = v.(string)
	} else {
		C.db = "lognile.db"
	}

	pattern, ok := data["pattern"]
	if !ok {
		panic("没有配置日志监听路径")	
	}

	C.parse(pattern)

    return C
}

func (C *Config) Db() string {
	if C.db=="" {
		panic("未执行初始化方法")
	}

	return C.db
}

func (C *Config) Pattern() map[string][]string {
	if C.patterns==nil {
		panic("未执行初始化方法")
	}

	return C.patterns
}

func (C *Config) parse(pattern any) {
	C.patterns = map[string][]string{}

	for _, p := range pattern.([]any) {
		abs, err := filepath.Abs(p.(string))
		if err!=nil {
			fmt.Fprintf(os.Stderr, "文件路径转绝对路径失败,file:"+p.(string)+",error:"+err.Error())
			continue
		}

		dir  := filepath.Dir(abs)
		base := filepath.Base(abs)

		if _, ok := C.patterns[dir];ok {
			C.patterns[dir] = append(C.patterns[dir], base)
		} else {
			C.patterns[dir] = []string{base}
		}
	}
}

