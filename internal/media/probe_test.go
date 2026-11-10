package media

import "testing"

func TestParseProbe(t *testing.T) {
	data := []byte(`{"streams":[
		{"codec_type":"video","codec_name":"H264","pix_fmt":"yuv420p","width":1080,"height":1920,"duration":"N/A"},
		{"codec_type":"audio","codec_name":"aac","duration":"9.5"},
		{"codec_type":"video","codec_name":"mjpeg","width":10,"height":10,"duration":"3"}
	],"format":{"duration":"10.25"}}`)
	got, err := parseProbe(data)
	if err != nil {
		t.Fatal(err)
	}
	want := Info{VideoCodec: "h264", PixFmt: "yuv420p", AudioCodec: "aac", Width: 1080, Height: 1920,
		VideoDuration: 3, AudioDuration: 9.5, FormatDuration: 10.25}
	if *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}

	got, err = parseProbe([]byte(`{"streams":[{"codec_type":"audio","codec_name":"mp3"}],"format":{"duration":"4"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoDuration != 4 || got.AudioDuration != 4 || got.VideoCodec != "" {
		t.Fatalf("format fallback: %+v", *got)
	}
	if _, err := parseProbe([]byte("nope")); err == nil {
		t.Fatal("want error")
	}
}
