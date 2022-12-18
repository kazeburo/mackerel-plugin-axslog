package ltsvreader

import (
	"bytes"
	"testing"

	"github.com/kazeburo/mackerel-plugin-axslog/axslog"
)

func TestParse(t *testing.T) {
	r := New("reqtime", []string{"status"})
	i, rt, st := r.Parse([]byte("time:08/Mar/2017:14:12:40 +0900	status:200	reqtime:0.030	host:10.20.30.40	req:GET /example/path HTTP/1.1	method:GET	size:941	ua:Mozilla/5.0 (Linux; Android 4.4.2; SO-01F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.90 Mobile Safari/537.36"))
	if i != axslog.AllFlagOK {
		t.Error("allflag not ok")
	}
	if !bytes.Equal(rt, []byte("0.030")) {
		t.Error("reqtime is not 0.030", string(rt))
	}
	if !bytes.Equal(st, []byte("200")) {
		t.Error("status is not 200", string(st))
	}
}

func TestParseHyphen(t *testing.T) {
	r := New("request_time", []string{"status"})
	i, rt, st := r.Parse([]byte("time:2022-12-14T03:29:26+09:00	host:0.0.0.0	remote_addr:1.1.1.1	status:200	request_time:0.001	referer:-	user_agent:curl/7.81.0	upstream_addr:2.2.2.2:443	upstream_response_time:0.001"))
	if i != axslog.AllFlagOK {
		t.Error("allflag not ok")
	}
	if !bytes.Equal(rt, []byte("0.001")) {
		t.Error("reqtime is not 0.030", string(rt))
	}
	if !bytes.Equal(st, []byte("200")) {
		t.Error("status is not 200", string(st))
	}
}

func TestParseNull(t *testing.T) {
	r := New("reqtime", []string{"status"})
	i, rt, st := r.Parse([]byte("time:08/Mar/2017:14:12:40 +0900	status:	reqtime:-	host:10.20.30.40	req:GET /example/path HTTP/1.1	method:GET	size:941	ua:Mozilla/5.0 (Linux; Android 4.4.2; SO-01F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.90 Mobile Safari/537.36"))
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
