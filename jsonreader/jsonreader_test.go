package jsonreader

import (
	"bytes"
	"testing"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

func TestParse(t *testing.T) {
	r := New("reqtime", []string{"status"})
	i, rt, st := r.Parse([]byte(`{"status":200,"reqtime":0.03,"size":941,"host":"10.20.30.40","req":"GET /example/path HTTP/1.1","time":"08/Mar/2017:14:12:40 +0900","ua":"Mozilla/5.0 (Linux; Android 4.4.2; SO-01F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.90 Mobile Safari/537.36","method":"GET"}"`))
	if i != axslog.AllFlagOK {
		t.Error("allflag not ok")
	}
	if !bytes.Equal(rt, []byte("0.03")) {
		t.Error("reqtime is not 0.03", string(rt))
	}
	if !bytes.Equal(st, []byte("200")) {
		t.Error("status is not 200", string(st))
	}
}

func TestParseNull(t *testing.T) {
	r := New("reqtime", []string{"status"})
	i, rt, st := r.Parse([]byte(`{"status":"-","reqtime":"","size":941,"host":"10.20.30.40","req":"GET /example/path HTTP/1.1","time":"08/Mar/2017:14:12:40 +0900","ua":"Mozilla/5.0 (Linux; Android 4.4.2; SO-01F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.90 Mobile Safari/537.36","method":"GET"}"`))
	if i == axslog.AllFlagOK {
		t.Error("allflag should be not ok")
	}
	if !bytes.Equal(rt, []byte("")) {
		t.Error("reqtime is not null", string(rt))
	}
	if !bytes.Equal(st, []byte("")) {
		t.Error("status is not null", string(st))
	}
}
