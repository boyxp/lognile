# Lognile
Golang实现的轻量级文件日志采集工具

![](https://img.shields.io/npm/l/vue.svg)

## 快速上手
创建配置文件config.yaml
```yaml
#要监听的日志路径
pattern :
    - ./_log/php-fpm-*.log
    - ./_log/nginx-*.log

#日志进度存储文件
db : lognile.db

```

启动程序加载配置，即可监听对应文件的日志新记录，新日志自动回调
```golang
package main

import "log"
import "github.com/boyxp/lognile"

func main() {
	lognile.Init("config.yaml", func(row map[string]string) {
		log.Println(row)
	})
}
```

