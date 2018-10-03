package main

import (
	"syscall"
	"os/signal"
	"os"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/pions/webrtc"
	"github.com/pions/webrtc/pkg/datachannel"
	"github.com/pions/webrtc/pkg/ice"

	"github.com/cocoonlife/goalsa"
	"gopkg.in/hraban/opus.v2"
)

var ctrlc = make(chan os.Signal)

func main() {
	addr := flag.String("address", ":50000", "Address that the HTTP server is hosted on.")
	flag.Parse()

	const channels = 2
	const sampleRate = 48000
	const format = alsa.FormatS16LE
	const pcmsize = 480

	player, err := alsa.NewPlaybackDevice("default", channels, format, sampleRate,
		alsa.BufferParams{BufferFrames: 0, PeriodFrames: pcmsize, Periods: pcmsize})
	if err != nil {
		panic(err)
	}
	defer player.Close()

	recorder, err := alsa.NewCaptureDevice("default", channels, format, sampleRate,
		alsa.BufferParams{BufferFrames: 0, PeriodFrames: pcmsize, Periods: pcmsize})
	if err != nil {
		panic(err)
	}
	defer recorder.Close()

	signal.Notify(ctrlc, os.Interrupt, syscall.SIGTERM)
	go cleanup(player, recorder)

	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		panic(err)
	}

	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		panic(err)
	}

	// Prepare the configuration
	config := webrtc.RTCConfiguration{
		IceServers: []webrtc.RTCIceServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.New(config)
	check(err)

	// Create a datachannel with label 'data'
	dataChannel, err := peerConnection.CreateDataChannel("data", nil)
	check(err)

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange = func(connectionState ice.ConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	}

	dataChannel.Lock()

	// Register channel opening handling
	dataChannel.OnOpen = func() {
		fmt.Printf("Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n", dataChannel.Label, dataChannel.ID)

		pcm := make([]int16, pcmsize)
		opusdata := make([]byte, 1000)

		for {
			recorder.Read(pcm)

			n, err := enc.Encode(pcm, opusdata) // pcm to opus
			if err != nil {
				panic(err)
			}
			opusdata = opusdata[:n] // Remove unused space after encoding to opus

			err = dataChannel.Send(datachannel.PayloadBinary{Data: opusdata})
			check(err)

			// fmt.Printf("Sending %s \n", message)
		}
	}

	// Register the Onmessage to handle incoming messages
	dataChannel.Onmessage = func(payload datachannel.Payload) {
		pcm := make([]int16, pcmsize)

		switch p := payload.(type) {
		case *datachannel.PayloadString:
			fmt.Printf("Message '%s' from DataChannel '%s' payload '%s'\n", p.PayloadType().String(), dataChannel.Label, string(p.Data))
		case *datachannel.PayloadBinary:
			// fmt.Printf("Message '%s' from DataChannel '%s' payload '% 02x'\n", p.PayloadType().String(), dataChannel.Label, p.Data)
			_, err := dec.Decode(p.Data, pcm)
			if err != nil {
				panic(err)
			}
			fmt.Println(p.Data)
			player.Write(pcm)
		default:
			fmt.Printf("Message '%s' from DataChannel '%s' no payload \n", p.PayloadType().String(), dataChannel.Label)
		}
	}

	dataChannel.Unlock()

	// Create an offer to send to the browser
	offer, err := peerConnection.CreateOffer(nil)
	check(err)

	// Exchange the offer for the answer
	answer := mustSignalViaHTTP(offer, *addr)

	// Apply the answer as the remote description
	err = peerConnection.SetRemoteDescription(answer)
	check(err)

	// Block forever
	select {}
}

// mustSignalViaHTTP exchange the SDP offer and answer using an HTTP Post request.
func mustSignalViaHTTP(offer webrtc.RTCSessionDescription, address string) webrtc.RTCSessionDescription {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(offer)
	check(err)

	resp, err := http.Post("http://"+address, "application/json; charset=utf-8", b)
	check(err)
	defer resp.Body.Close()

	var answer webrtc.RTCSessionDescription
	err = json.NewDecoder(resp.Body).Decode(&answer)
	check(err)

	return answer
}

func randSeq(n int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

// check is used to panic in an error occurs.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func cleanup(player *alsa.PlaybackDevice, recorder *alsa.CaptureDevice) {
	// User hit Ctrl-C, clean up
	<-ctrlc
	fmt.Println("Close devices")
	player.Close()
	recorder.Close()
	os.Exit(1)
}
