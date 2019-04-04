package logFile

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

var Log *logs.BeeLogger

type logArgs struct {
	FileName string `json:"filename"`
	Maxlines int    `json:"maxlines"`
}


func InitLog() {
	logPath := beego.AppConfig.String("logs::path")
	logName :=filepath.Join(logPath,"log_" + time.Now().Format("2006_01") + ".log")
	args := logArgs{
		FileName: logName,
		Maxlines:10000,
	}

	str, err := json.Marshal(args)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = logs.SetLogger(logs.AdapterFile,string(str))
	if err != nil {
		fmt.Println(err)
		return
	}

	//log打印文件名和行数
	//logs.SetLogFuncCall(true)
	logs.GetBeeLogger().DelLogger("console")
	logs.Info("program starting\r")
}





