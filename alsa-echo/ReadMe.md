#### Dependencies for alsa:

`sudo apt install libasound2-dev`

Test your alsa device is working or not,

You should hear your own voice after running it!

If not, I guess you need to set the value of:

| Name                              |  default          |
| --------------------------------- | ----------------- |
| Alsa device name                  |  "default"        |
| channels                          |  1 or 2           |
| sampleRate                        |  8000/16000/48000 |
| pcmsize                           |  80/160/480/960   |

You can have a full list of alsa device names by using these commands:

```sh
aplay -l
aplay -L
arecord -l
arecord -L
```
