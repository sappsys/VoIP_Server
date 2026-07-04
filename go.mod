module github.com/sappsys/VoIP_Server

go 1.26.4

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/CyCoreSystems/goertzel v0.1.0
	github.com/Distortions81/g726 v0.0.8
	github.com/emiago/diago v0.29.0
	github.com/emiago/sipgo v1.4.0
	github.com/gotranspile/g722 v0.0.0-20240123003956-384a1bb16a19
	github.com/hajimehoshi/go-mp3 v0.3.4
	github.com/hunydev/g729 v0.2.3-rc5
	github.com/pion/stun/v3 v3.1.6
	golang.org/x/crypto v0.48.0
	modernc.org/sqlite v1.34.5
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emiago/dtls/v3 v3.0.0-20260122183559-8b8d23e359c0 // indirect
	github.com/go-audio/riff v1.0.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/icholy/digest v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/pion/dtls/v3 v3.1.4 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.9.0 // indirect
	github.com/pion/srtp/v3 v3.0.6 // indirect
	github.com/pion/transport/v3 v3.1.1 // indirect
	github.com/pion/transport/v4 v4.0.2 // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/zaf/g711 v1.4.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	gopkg.in/hraban/opus.v2 v2.0.0-20230925203106-0188a62cb302 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
)

replace github.com/emiago/diago => ./third_party/diago
