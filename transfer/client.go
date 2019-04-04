package transfer

import (
	"DirSyncSystem/monitor"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/astaxie/beego/logs"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/astaxie/beego"
)

var server string
var port string

//保存未上传成功的文件 1:上传 2:删除 3:下载
var fileCache map[string]int
//var SyncSignal chan bool


type Arg struct {
	FileList    map[string][]byte `json:"file_list"`
}

type Reply struct {
	RemoveFile   []string `json:"remove_file"`
	DownloadFile []string `json:"download_file"`
}

func init() {
	server = beego.AppConfig.String("server::ip")
	port = beego.AppConfig.String("server::port")

	fileCache = make(map[string]int)
	//SyncSignal = make(chan bool)
}

func SyncFile() {
	fmt.Println("同步开始")
	logs.Info("[同步开始]\r")
	//SyncSignal <- true

	arg := Arg{}
	arg.FileList = make(map[string][]byte)

	//获取目录下的全部文件列表
	filepath.Walk(monitor.DirPath,func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			path, err := filepath.Abs(path)
			if err != nil {
				logs.Error("[SET ABS PATH ERROR]\r")
				return err
			}
			uploadPath := strings.TrimPrefix(path, monitor.DirPath)
			uploadPath = uploadPath[1:]
			if runtime.GOOS == "windows" {
				uploadPath = strings.Replace(uploadPath, "\\", "/", -1)
			}

			//利用文件的md5值判断本地文件与服务器文件是否相同
			f,err := os.Open(path)
			if err != nil {
				fmt.Println("md5 open ",err)
				logs.Error("[MD5 OPEN ERROR] [%s]\r",path)
				return err
			}
			md5hash := md5.New()
			if _,err := io.Copy(md5hash,f);err == nil{
				arg.FileList[uploadPath] = md5hash.Sum(nil)
			}else {
				logs.Error("[md5 ERROR] [%s]\r",path)
				fmt.Println("md5 error",err)
			}
			f.Close()

			//arg.FileList[uploadPath] = info.ModTime()
		}
		return nil
	})


	//上传目录列表到服务器
	var conn net.Conn
	var err error
	argJson,err := json.Marshal(arg)
	if err != nil {
		logs.Error("[ARG JSON ENCODE ERROR] [%s]\r",arg)
		logs.Error("[同步失败]")
		fmt.Println("同步失败")
		//SyncSignal <- true
		return
	}
	for times := 0;;times ++{
		conn, err = net.DialTimeout("tcp", server+":"+port, 10*time.Second)
		if err != nil {
			fmt.Println(err)
			time.Sleep(10 * time.Second)
			if times == 9 {
				logs.Warn("[no connection]\r")
				fmt.Println("无网络连接")
				fmt.Println("同步失败")
				//SyncSignal <- true
				return
			}
		} else {
			break
		}
	}
	defer conn.Close()

	fmt.Println("与服务器连接成功")
	lenOfArgJson := len(argJson)
	buff := make([]byte,1024*4)

	_,err = conn.Write([]byte("sync"))
	if err != nil {
		logs.Error("[同步失败] [%s]\r",err)
		fmt.Println("同步失败：",err)
		//SyncSignal <- true
		return
	}

	size,err := conn.Read(buff)
	if err != nil {
		logs.Error("[同步失败] [%s]\r",err)
		fmt.Println("同步失败：",err)
		//SyncSignal <- true
		return
	}
	if "ok" != string(buff[:size]) {
		logs.Error("[同步失败] [%s]\r",err)
		fmt.Println("同步失败：",err)
		//SyncSignal <- true
		return
	}

	n := lenOfArgJson/cap(buff)
	sum := 0
	for i := 0;i <= n;i++{
		if i != n {
			_,err = conn.Write(argJson[sum:sum + 1024*4])
		}else {
			_,err = conn.Write(argJson[sum:])
		}
		//_,err = conn.Write(argJson[sum:sum + 1024*4])
		if err != nil {
			logs.Error("[同步失败] [%s]\r",err)
			fmt.Println("同步失败：",err)
			//SyncSignal <- true
			return
		}
		sum = sum + 1024*4
	}
	_,err = conn.Write([]byte("end"))
	if err != nil {
		logs.Error("[同步失败] [%s]\r",err)
		fmt.Println("同步失败：",err)
		//SyncSignal <- true
		return
	}

	//接收服务器传来的回复（待上传文件列表与待下载文件列表）
	sumBuff := ""
	for {
		size,err = conn.Read(buff)
		if err != nil && err != io.EOF{
			logs.Error("[同步失败] [%s]\r",err)
			fmt.Println("同步失败：",err)
			//SyncSignal <- true
			return
		}
		if string(buff[size-3:size]) == "end" {
			sumBuff = sumBuff + string(buff[:size-3])
			break
		}else {
			sumBuff = sumBuff + string(buff[:size])
		}
	}
	//size,err = conn.Read(buff)
	//if err != nil {
	//	logs.Error("[同步失败] [%s]\r",err)
	//	fmt.Println("同步失败：",err)
	//	SyncSignal <- true
	//	return
	//}
	reply := Reply{}
	err = json.Unmarshal([]byte(sumBuff),&reply)
	if err != nil {
		logs.Error("[REPLY JSON DISCODE ERROR] [%s]\r",reply)
		logs.Error("[同步失败] [%s]\r",err)
		fmt.Println("同步失败：",err)
		//SyncSignal <- true
		return
	}

	var wg sync.WaitGroup

	//遍历删除文件列表，删除
	for _,df := range reply.RemoveFile{
		if runtime.GOOS == "windows"{
			df = strings.Replace(df,"/","\\",-1)
		}
		df = filepath.Join(monitor.DirPath,df)
		err = os.RemoveAll(df)
		if err != nil{
			fmt.Println(err)
		}
	}

	//遍历下载文件列表，下载
	for _,df := range reply.DownloadFile{
		wg.Add(1)
		go func(df string) {
			defer wg.Done()
			TcpDownloadFile(df)
		}(df)
	}

	wg.Wait()

	logs.Warn("[同步完成]\r")
	fmt.Println("同步完成")
	//SyncSignal <- true
}


//上传文件
func TcpUploadFile(path string) {
	if runtime.GOOS == "windows" {
		ignoreFile := path[strings.LastIndex(path,"\\")+1:]
		if strings.HasPrefix(ignoreFile,"~") ||strings.HasPrefix(ignoreFile,"."){
			return
		}
	}else {
		ignoreFile := path[strings.LastIndex(path,"/")+1:]
		if strings.HasPrefix(ignoreFile,"~")||strings.HasPrefix(ignoreFile,"."){
			return
		}
	}


	logs.Warn("[上传文件] [%s]\r",path)
	fmt.Println("上传文件：" + path)
	var conn net.Conn
	var err error

	for i := 0;i < 10;i++{
		conn, err = net.DialTimeout("tcp", server+":"+port, 10*time.Second)
		if err != nil {
			fmt.Println(err)
			time.Sleep(10 * time.Second)
			if i == 9 {
				logs.Warn("[no connection] [add cache] [%s]\r",path)
				fmt.Println("无网络连接")
				fileCache[path] = 1
				return
			}
		} else {
			break
		}
	}
	defer conn.Close()

	buf := make([]byte, 1024*4)
	_,err = conn.Write([]byte("create"))
	if err != nil {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错:",err)
		fileCache[path] = 1
		return
	}
	size, err := conn.Read(buf)
	if err != nil {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错:",err)
		fileCache[path] = 1
		return
	}
	if "ok" != string(buf[:size]) {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错")
		fileCache[path] = 1
		return
	}

	//传送文件名称
	var uploadPath string
	if runtime.GOOS == "windows" {
		uploadPath = strings.TrimPrefix(path, monitor.DirPath+"\\")
		uploadPath = strings.Replace(uploadPath, "\\", "/", -1)
	}else {
		uploadPath = strings.TrimPrefix(path,monitor.DirPath + "/")
	}
	_,err = conn.Write([]byte(uploadPath))
	if err != nil {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错:",err)
		fileCache[path] = 1
		return
	}

	//接收文件
	size, err = conn.Read(buf)
	if err != nil {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错:",err)
		fileCache[path] = 1
		return
	}
	if "ok" != string(buf[:size]) {
		logs.Error("[上传出错] [%s]\r",path)
		fmt.Println("上传出错")
		fileCache[path] = 1
		return
	}

	file, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		logs.Error("[OPEN FILE ERROR] [%s] [%s]\r",path,err)
		fileCache[path] = 1
		return
	}
	defer file.Close()
	for {
		size, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("上传完毕：", path)
				logs.Warn("[上传完毕] [%s]\r",path)
			} else {
				logs.Error("[上传出错] [%s]\r",path)
				fmt.Println("上传出错:",err)
				fileCache[path] = 1
			}
			return
		}
		_,err =conn.Write(buf[:size])
		if err != nil {
			logs.Error("[上传出错] [%s]\r",path)
			fmt.Println("上传出错:",err)
			fileCache[path] = 1
			return
		}
	}

}

func TcpDeleteFile(path string) {
	logs.Warn("[删除文件] [%s]\r",path)
	fmt.Println("删除文件：", path)
	var conn net.Conn
	var err error

	for times := 0;;times ++{
		conn, err = net.DialTimeout("tcp", server+":"+port, 10*time.Second)
		if err != nil {
			fmt.Println(err)
			time.Sleep(10 * time.Second)
			if times == 9 {
				logs.Warn("[no connection] [add cache] [%s]\r",path)
				fmt.Println("无网络连接")
				fileCache[path] = 2
				return
			}
		} else {
			break
		}
	}
	defer conn.Close()

	buf := make([]byte, 1024*4)
	_,err = conn.Write([]byte("remove"))
	if err != nil {
		fmt.Println("删除失败：",err)
		logs.Error("[删除失败] [%s]\r",path)
		fileCache[path] = 2
		return
	}

	size, err := conn.Read(buf)
	if err != nil {
		fmt.Println("删除失败：",err)
		logs.Error("[删除失败] [%s]\r",path)
		fileCache[path] = 2
		return
	}
	if "ok" != string(buf[:size]) {
		fmt.Println("删除失败")
		logs.Error("[删除失败] [%s]\r",path)
		fileCache[path] = 2
		return
	}

	var removePath string
	if runtime.GOOS == "windows" {
		removePath = strings.TrimPrefix(path,monitor.DirPath + "\\")
		removePath = strings.Replace(removePath,"\\","/",-1)
	}else {
		removePath = strings.TrimPrefix(path,monitor.DirPath + "/")
	}
	_, err = conn.Write([]byte(removePath))
	if err != nil {
		fmt.Println("删除失败：",err)
		logs.Error("[删除失败] [%s]\r",path)
		fileCache[path] = 2
		return
	}
}


func TcpDownloadFile(path string) {
	//本地文件地址
	var savePath string
	//目录地址
	var dirPath string

	switch runtime.GOOS {
	case "darwin":
		savePath = filepath.Join(monitor.DirPath,path)
		fmt.Println("下载文件：",savePath)
		logs.Warn("[下载文件] [%s]\r",savePath)
		dirPath = path[:strings.LastIndex(path,"/")+1]
	case "windows":
		savePath = filepath.Join(monitor.DirPath,strings.Replace(path,"/","\\",-1))
		fmt.Println("下载文件：",savePath)
		logs.Warn("[下载文件] [%s]\r",savePath)
		dirPath = path[:strings.LastIndex(strings.Replace(path,"/","\\",-1),"\\")+1]
	default:
		savePath = filepath.Join(monitor.DirPath,path)
		fmt.Println("下载文件：",savePath)
		logs.Warn("[下载文件] [%s]\r",savePath)
		dirPath = path[:strings.LastIndex(path,"/")+1]
	}

	var conn net.Conn
	var err error
	for times := 0;;times ++{
		conn, err = net.DialTimeout("tcp", server+":"+port, 10*time.Second)
		if err != nil {
			fmt.Println(err)
			time.Sleep(10 * time.Second)
			if times == 9 {
				logs.Warn("[no connection] [add cache] [%s]\r",path)
				fmt.Println("无网络连接")
				fileCache[path] = 3
				return
			}
		} else {
			break
		}
	}
	defer conn.Close()

	buff := make([]byte,1024*4)
	_,err = conn.Write([]byte("download"))
	if err != nil {
		fmt.Println("下载失败：",err)
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}

	size,err := conn.Read(buff)
	if err != nil {
		fmt.Println("下载失败：",err)
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}
	if "ok" != string(buff[:size]){
		fmt.Println("下载失败")
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}


	//发送需要下载的文件
	_,err = conn.Write([]byte(path))
	if err != nil {
		fmt.Println("下载失败：",err)
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}

	//建立下载文件
	err = os.MkdirAll(filepath.Join(monitor.DirPath,dirPath),os.ModePerm)
	if err != nil {
		fmt.Println("下载失败：",err)
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}
	file,err := os.Create(savePath)
	defer file.Close()
	if err != nil {
		fmt.Println("下载失败：",err)
		logs.Error("[下载失败] [%s]\r",savePath)
		fileCache[path] = 3
		return
	}

	//下载文件
	for {
		size,err := conn.Read(buff)
		if err != nil{
			if err == io.EOF{
				fmt.Println("下载完成：",savePath)
				logs.Warn("[下载完成] [%s]\r",savePath)
			}else{
				fmt.Println("下载失败：",err)
				logs.Error("[下载失败] [%s]\r",savePath)
				fileCache[path] = 3
				return
			}
			return
		}
		_,err = file.Write(buff[:size])
		if err != nil {
			fmt.Println("下载失败：",err)
			logs.Error("[下载失败] [%s]\r",savePath)
			fileCache[path] = 3
			return
		}
	}

}

func TimeClean(){
	select {
	case <- time.After(15 * time.Minute):
		for path,state := range fileCache {
			switch state {
			case 1:{
				delete(fileCache,path)
				go TcpUploadFile(path)
			}
			case 2:{
				delete(fileCache,path)
				go TcpDeleteFile(path)
			}
			case 3:{
				delete(fileCache,path)
				go TcpDownloadFile(path)
			}
			}
		}
	}
}