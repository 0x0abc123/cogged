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
	fmt.Println(time.Now(), strings.TrimSpace(msg), objsSerialised)
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