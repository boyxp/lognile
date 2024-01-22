package lognile

import "os"
import "log"
import "sync"
import "encoding/json"

type Offset struct {
	db string
}

func (O *Offset) Load() sync.Map {
	if O.db=="" {
		log.Fatal("db文件名不可为空")
	}

	var result sync.Map

	_, err := os.Stat(O.db)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		log.Fatal("db文件加载失败:", err)
	}

	content, err := os.ReadFile(O.db)
	if err != nil {
		log.Fatal("db文件读取失败:", err)
	}

	var offset map[uint64]int64
	err = json.Unmarshal(content, &offset)
	if err != nil {
		log.Fatal("db文件解析失败:", err)
	}

	for _node,_offset := range offset {
		result.Store(_node, _offset)
	}

	return result
}

func (O *Offset) Save(offset map[uint64]int64) {
	if O.db=="" {
		log.Fatal("db文件名不可为空")
	}

	content, err := json.Marshal(offset)
	if err != nil {
		log.Println("请手动保存进度数据：", offset)
		log.Fatal("进度数据编码失败:", err)
	}

	if err := os.WriteFile(O.db, []byte(content), 0666); err != nil {
		log.Println("请手动保存进度数据：", content)
		log.Fatal("读取进度数据存储失败:", err)
	}
}
