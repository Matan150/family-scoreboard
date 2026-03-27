module github.com/matan150/family-scoreboard/listener

go 1.26.1

// Add this line to point to your local Whisper C++ code
replace github.com/ggerganov/whisper.cpp/bindings/go => ./whisper.cpp/bindings/go

require github.com/Picovoice/pvrecorder/binding/go v1.2.2

require github.com/ggerganov/whisper.cpp/bindings/go v0.0.0-20260321170300-76684141a5d0

require (
	github.com/gorilla/websocket v1.5.3
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/text v0.20.0 // indirect
)
