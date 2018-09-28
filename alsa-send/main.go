package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"bufio"
	"encoding/base64"

	"github.com/cocoonlife/goalsa"
	"github.com/pions/webrtc"

	// "github.com/pions/webrtc/examples/gstreamer-send/gst"
	"gopkg.in/hraban/opus.v2"
	"github.com/pions/webrtc/pkg/ice"
	"github.com/pions/webrtc/pkg/media"
	"github.com/pions/webrtc/pkg/rtp"
)

var ctrlc = make(chan os.Signal)

func main() {
	reader := bufio.NewReader(os.Stdin)
	rawSd, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		panic(err)
	}

	fmt.Println("")
	sd, err := base64.StdEncoding.DecodeString(rawSd)
	if err != nil {
		panic(err)
	}

	const channels = 2
	const sampleRate = 48000
	const format = alsa.FormatS16LE
	const pcmsize = 480
	const mtu = 1400
	PayloadTypeOpus := uint8(webrtc.DefaultPayloadTypeOpus)
	opuscodec := webrtc.NewRTCRtpOpusCodec(PayloadTypeOpus, sampleRate, channels)

	/* Everything below is the pion-WebRTC API, thanks for using it! */

	// Setup the codecs you want to use.
	// We'll use the default ones but you can also define your own
	webrtc.RegisterCodec(opuscodec)

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.New(webrtc.RTCConfiguration{
		IceServers: []webrtc.RTCIceServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange = func(connectionState ice.ConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	}

	// Create a audio track
	opusTrack, err := peerConnection.NewRTCTrack(webrtc.DefaultPayloadTypeOpus, "audio", "pion1")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(opusTrack)
	if err != nil {
		panic(err)
	}

	// Create a video track
	// vp8Track, err := peerConnection.NewRTCTrack(webrtc.DefaultPayloadTypeVP8, "video", "pion2")
	// if err != nil {
	// 	panic(err)
	// }
	// _, err = peerConnection.AddTrack(vp8Track)
	// if err != nil {
	// 	panic(err)
	// }

	// Set the remote SessionDescription
	offer := webrtc.RTCSessionDescription{
		Type: webrtc.RTCSdpTypeOffer,
		Sdp:  string(sd),
	}
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Get the LocalDescription and take it to base64 so we can paste in browser
	fmt.Println(base64.StdEncoding.EncodeToString([]byte(answer.Sdp)))

	// Start pushing buffers on these tracks

	// pcm := make(chan []int16)

	recorder, err := alsa.NewCaptureDevice("default", channels, format, sampleRate,
		alsa.BufferParams{BufferFrames: 0, PeriodFrames: pcmsize, Periods: pcmsize})

	// player, err := alsa.NewPlaybackDevice("hw:0,0", channels, format, sampleRate,
	// 	alsa.BufferParams{BufferFrames: 0, PeriodFrames: bufsize, Periods: bufsize})

	if err != nil {
		panic(err)
	}
	defer recorder.Close()
	signal.Notify(ctrlc, os.Interrupt, syscall.SIGTERM)
	go cleanup(recorder)

	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		panic(err)
	}

	pcm := make(chan []int16)

	/* Recording audio */
	go func() {
		p := make([]int16, pcmsize)
		for {
			recorder.Read(p)
			pcm <- p
		}
	}()

	/* Send audio */
	go func() {
		p := make([]int16, pcmsize)
		opusdata := make([]byte, 1000)

		ran := make([]byte, 4)
		_, err = rand.Read(ran)
		if err != nil {
			panic(err)
		}

		ssrc := binary.LittleEndian.Uint32(ran)

		packetizer := rtp.NewPacketizer(
			mtu,
			opuscodec.PayloadType,
			ssrc,
			opuscodec.Payloader,
			rtp.NewRandomSequencer(),
			sampleRate,
		)

		for {
			p = <-pcm

			n, err := enc.Encode(p, opusdata) // pcm to opus
			if err != nil {
				panic(err)
			}
			opusdata = opusdata[:n] // Remove unused space after encoding to opus

			packets := packetizer.Packetize(opusdata, pcmsize)

			raw, err := packets[0].Marshal()
			if err != nil {
				panic(err)
			}

			fmt.Println(len(raw))

			opusTrack.Samples <- media.RTCSample{Data: raw, Samples: uint32(pcmsize)}
		}
	}()

	// gst.CreatePipeline(webrtc.Opus, opusTrack.Samples).Start()
	// gst.CreatePipeline(webrtc.VP8, vp8Track.Samples).Start()
	select {}
}

func cleanup(recorder *alsa.CaptureDevice) {
	// User hit Ctrl-C, clean up
	<-ctrlc
	fmt.Println("Close devices")
	recorder.Close()
	os.Exit(1)
}