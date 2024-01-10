package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type persistRequest struct {
	filename string
	data     []byte
}

func persister(pChan <-chan persistRequest, doneChan chan<- struct{}) {
	for req := range pChan {
		f, err := os.Create(fmt.Sprintf("dumps/%s", req.filename))
		if err != nil {
			fmt.Printf("Failed to create dumps/%s\n: %s", req.filename, err)
			continue
		}
		defer f.Close()

		_, err = f.Write(req.data)
		if err != nil {
			fmt.Printf("Failed to write dumps/%s\n: %s", req.filename, err)
			continue
		}
		f.Close()
	}

	doneChan <- struct{}{}
}

type convertRequest struct {
	data []byte
	id   int
}

func converter(id int, wg *sync.WaitGroup, reqChan <-chan convertRequest, persistChan chan persistRequest) {
	for req := range reqChan {
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
	wg.Done()
}

func main() {
	numConverters := runtime.NumCPU() - 1

	doneChan := make(chan struct{})
	persistChan := make(chan persistRequest, 100)
	go persister(persistChan, doneChan)

	var convertWg sync.WaitGroup
	convertChan := make(chan convertRequest, 100)
	for i := 0; i < numConverters; i++ {
		convertWg.Add(1)
		go converter(i, &convertWg, convertChan, persistChan)
	}

	X, err := xgb.NewConn()
	if err != nil {
		fmt.Println(err)
		return
	}
	screen := xproto.Setup(X).DefaultScreen(X)

	for i := 0; i < 100; i++ {
		cookie := xproto.GetImage(X, xproto.ImageFormatZPixmap, xproto.Drawable(screen.Root), 0, 0, 1920, 1080, 0xffffffff)
		fmt.Println(cookie)
		ximg, err := cookie.Reply()
		if err != nil {
			fmt.Printf("cookie reply failed: %s\n", err)
			continue
		}

		convertChan <- convertRequest{ximg.Data, i}
	}

	close(convertChan)
	convertWg.Wait()
	close(persistChan)

	<-doneChan

	X.Close()
}
