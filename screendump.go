package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	//"os/signal"
	"runtime"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

type persistRequest struct {
	filename string
	data     []byte
}

func persister(pChan <-chan persistRequest) {
	for {
		req := <-pChan
		f, err := os.Create(fmt.Sprintf("dumps/%s", req.filename))
		if err == nil {
			f.Write(req.data)
			f.Close()
		}
	}
}

type convertRequest struct {
	data []byte //*xproto.GetImageReply
	id   int
}

func converter(id int, reqChan <-chan convertRequest, persistChan chan<- persistRequest) {
	for {
		req := <-reqChan
		data := req.data
		for i := 0; i < len(data); i += 4 {
			data[i], data[i+2], data[i+3] = data[i+2], data[i], 255
		}
		img := &image.RGBA{data, 4 * 1920, image.Rect(0, 0, 1920, 1080)}
		buf := new(bytes.Buffer)
		err := png.Encode(buf, img)
		if err == nil {
			persistChan <- persistRequest{fmt.Sprintf("dump%d (worker %d).png", req.id, id), buf.Bytes()}
		} else {
			fmt.Println("encode error", err)
		}
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	captureChan := make(chan []byte) //*xproto.GetImageReply)

	persistChan := make(chan persistRequest, 100)
	go persister(persistChan)

	convertChan := make(chan convertRequest, 100)
	go converter(1, convertChan, persistChan)
	go converter(2, convertChan, persistChan)
	go converter(3, convertChan, persistChan)
	go converter(4, convertChan, persistChan)
	go converter(5, convertChan, persistChan)
	go converter(6, convertChan, persistChan)

	done := false
	go func() {
		for count := 0; count < 100; count++ {
			imagereply := <-captureChan
			convertChan <- convertRequest{imagereply, count}
		}
		done = true
	}()

	X, err := xgb.NewConn()
	if err != nil {
		return
	}
	screen := xproto.Setup(X).DefaultScreen(X)

	// I don't know wtf but it seems like XGB uses channels internally to manage the reply
	// cookies and the first call to reply always seem to cause a data race. This is obviously
	// uncool
	wtf := xproto.GetImage(X, xproto.ImageFormatZPixmap, xproto.Drawable(screen.Root), 0, 0, 1920, 1080, 0xffffffff)
	time.Sleep(1 * time.Second)
	wtf.Reply()

	for !done {
		cookie := xproto.GetImage(X, xproto.ImageFormatZPixmap, xproto.Drawable(screen.Root), 0, 0, 1920, 1080, 0xffffffff)
		fmt.Println(cookie)
		ximg, err := cookie.Reply()
		if err == nil {
			captureChan <- ximg.Data
		}
	}

	X.Close()
}
