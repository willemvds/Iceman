package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/willemvds/Iceman"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type persistRequest struct {
	filename string
	data     []byte
}

func persister(pChan <-chan persistRequest, doneChan chan<- struct{}) {
	batchId := fmt.Sprintf("%d", time.Now().UnixNano())
	err := os.MkdirAll(batchId, 0755)
	fmt.Println("batchId dir err", batchId, err)
	for req := range pChan {
		f, err := os.Create(fmt.Sprintf("%s/%s", batchId, req.filename))
		if err != nil {
			fmt.Printf("Failed to create screenshot/%s\n: %s", req.filename, err)
			continue
		}
		defer f.Close()

		_, err = f.Write(req.data)
		if err != nil {
			fmt.Printf("Failed to write screenshot/%s\n: %s", req.filename, err)
			continue
		}
		f.Close()
	}

	doneChan <- struct{}{}
}

type convertRequest struct {
	width  uint16
	height uint16
	data   []byte
	id     uint
}

func converter(id int, wg *sync.WaitGroup, reqChan <-chan convertRequest, persistChan chan persistRequest) {
	for req := range reqChan {
		data := req.data
		for i := 0; i < len(data); i += 4 {
			data[i], data[i+2], data[i+3] = data[i+2], data[i], 255
		}
		img := &image.RGBA{data, 4 * int(req.width), image.Rect(0, 0, int(req.width), int(req.height))}
		buf := new(bytes.Buffer)
		err := png.Encode(buf, img)
		if err != nil {
			fmt.Println("encode error", err)
			continue
		}
		persistChan <- persistRequest{fmt.Sprintf("%08d.png", req.id), buf.Bytes()}

	}
	wg.Done()
}

func main() {
	numConverters := runtime.NumCPU() - 2

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

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	sg := Iceman.NewScreenGrabber(ctx, screen, 30, 30)
	ssChan, errorsChan := sg.StartCapturing(X)

	go func() {
		for e := range errorsChan {
			fmt.Println("Screenshot Error", e)
		}
	}()

	for ss := range ssChan {
		fmt.Println("Screenshot #", ss.Index, ss.RequestedAt, ss.StartedAt, ss.ReceivedAt, ss.ReceivedAt.Sub(ss.RequestedAt), ss.ReceivedAt.Sub(ss.StartedAt))
		convertChan <- convertRequest{
			screen.WidthInPixels,
			screen.HeightInPixels,
			ss.ImageReply.Data,
			ss.Index,
		}
	}

	close(convertChan)
	convertWg.Wait()
	close(persistChan)

	<-doneChan

	X.Close()
}
