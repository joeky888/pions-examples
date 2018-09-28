package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joeky888/gplay/alsa"
)

var ctrlc = make(chan os.Signal)

func main() {
	fmt.Println("")
	const channels = 1
	const sampleRate = 16000
	const format = alsa.FormatS16LE
	pcm := make(chan []int16)

	player, err := alsa.NewPlaybackDevice("default", channels, format, sampleRate,
		alsa.BufferParams{BufferFrames: 0, PeriodFrames: 320, Periods: 320})

	if err != nil {
		panic(err)
	}
	defer player.Close()

	recorder, err := alsa.NewCaptureDevice("default", channels, format, sampleRate,
		alsa.BufferParams{BufferFrames: 0, PeriodFrames: 320, Periods: 320})

	if err != nil {
		panic(err)
	}
	defer recorder.Close()

	// defer player.Close()
	// defer recorder.Close()

	go func() {
		buf := make([]int16, 1024)
		for {
			recorder.Read(buf)
			pcm <- buf
		}
	}()

	go func() {
		buf := make([]int16, 1024)
		for {
			buf = <-pcm
			player.Write(buf)
		}
	}()

	signal.Notify(ctrlc, os.Interrupt, syscall.SIGTERM)
	go cleanup(player, recorder)
	select {}
}

func cleanup(player *alsa.PlaybackDevice, recorder *alsa.CaptureDevice) {
	// User hit Ctrl-C, clean up
	<-ctrlc
	fmt.Println("Close devices")
	player.Close()
	recorder.Close()
	os.Exit(1)
}
