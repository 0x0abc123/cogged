package log

import (
	"fmt"
	"time"
	"strings"
	"encoding/json"
)


func dump(msg string, obj ...interface{}) {
	objsSerialised := ""
	for _, o := range obj {
		if o != nil {
			j, _ := json.Marshal(o)
			objsSerialised += "," + string(j) 
		}	
	}
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	fmt.Println(timestamp, strings.TrimSpace(msg), objsSerialised)
}


func Debug(msg string, obj ...interface{}) {
	dump("DEBUG: "+msg, obj...)
}


func Info(msg string, obj ...interface{}) {
	dump("INFO: "+msg, obj...)
}


func Warn(msg string, obj ...interface{}) {
	dump("WARN: "+msg, obj...)
}


func Error(msg string, obj ...interface{}) {
	dump("ERROR: "+msg, obj...)
}