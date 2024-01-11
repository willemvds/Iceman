package Iceman

import (
	"context"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// ScreenGrabber tries to get screenshots at the rate specified.
type ScreenGrabber struct {
	ctx context.Context

	screen         *xproto.ScreenInfo
	snapsPerSecond uint
	maxSnaps       uint
}

func NewScreenGrabber(ctx context.Context, screen *xproto.ScreenInfo, snapsPerSecond uint, maxSnaps uint) *ScreenGrabber {
	return &ScreenGrabber{
		ctx,
		screen,
		snapsPerSecond,
		maxSnaps,
	}
}

func (sg *ScreenGrabber) StartCapturing(X *xgb.Conn) (chan *Screenshot, chan error) {
	requestChan := make(chan *Screenshot)
	errorsChan := make(chan error)
	responseChan := make(chan *Screenshot)

	go func(responseChan chan *Screenshot) {
		for ss := range requestChan {
			ss.StartedAt = time.Now()
			cookie := xproto.GetImage(X, xproto.ImageFormatZPixmap, xproto.Drawable(sg.screen.Root), 0, 0, sg.screen.WidthInPixels, sg.screen.HeightInPixels, 0xffffffff)
			ximg, err := cookie.Reply()
			if err != nil {
				errorsChan <- err
				continue
			}
			ss.ImageReply = ximg
			ss.ReceivedAt = time.Now()
			responseChan <- ss
		}
		close(errorsChan)
		close(responseChan)
	}(responseChan)

	go func() {
		snapDelayMs := time.Duration((1.0 / float64(sg.snapsPerSecond)) * 1e9)
		ticker := time.NewTicker(snapDelayMs)
		index := 0
		for {
			select {
			case <-sg.ctx.Done():
				close(requestChan)
				return
			case t := <-ticker.C:
				requestChan <- &Screenshot{
					Index:       uint(index),
					RequestedAt: t,
				}
				index += 1
			}
		}
	}()

	return responseChan, errorsChan
}

type Screenshot struct {
	Index       uint
	RequestedAt time.Time
	StartedAt   time.Time
	ReceivedAt  time.Time
	ImageReply  *xproto.GetImageReply
}
