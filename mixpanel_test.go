package mixpanel

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

var (
	ts          *httptest.Server
	client      Mixpanel
	LastRequest *http.Request
	LastPost    []byte
)

func setup() {
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("1\n"))
		LastRequest = r

		var err error
		LastPost, err = io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
	}))

	client = NewWithSecret("e3bc4100330c35722740fb8c6f5abddc", "mysecret", ts.URL)
}

func teardown() {
	ts.Close()
}

func decodeURL(url string) string {
	data := strings.Split(url, "data=")[1]
	decoded, _ := base64.StdEncoding.DecodeString(data)
	return string(decoded[:])
}

func decodeBody() string {
	data := string(LastPost)
	if strings.HasPrefix(data, "data=") {
		data = strings.Split(string(LastPost), "data=")[1]
		decoded, _ := base64.StdEncoding.DecodeString(data)
		data = string(decoded)
	}

	return data
}

// examples from https://mixpanel.com/help/reference/http

func TestTrack(t *testing.T) {
	setup()
	defer teardown()

	client.Track(context.TODO(), "13793", "Signed Up", &Event{
		Properties: map[string]interface{}{
			"Referred By": "Friend",
		},
	})

	want := "{\"event\":\"Signed Up\",\"properties\":{\"Referred By\":\"Friend\",\"distinct_id\":\"13793\",\"token\":\"e3bc4100330c35722740fb8c6f5abddc\"}}"

	if !reflect.DeepEqual(decodeBody(), want) {
		t.Errorf("Post body returned %+v, want %+v",
			decodeBody(), want)
	}

	want = "/track"
	path := LastRequest.URL.Path

	if !reflect.DeepEqual(path, want) {
		t.Errorf("path returned %+v, want %+v",
			path, want)
	}
}

func TestImport(t *testing.T) {
	setup()
	defer teardown()

	importTime := time.Now().Add(-5 * 24 * time.Hour)

	client.Import(context.TODO(), "13793", "Signed Up", &Event{
		Properties: map[string]interface{}{
			"Referred By": "Friend",
		},
		Timestamp: &importTime,
	})

	want := fmt.Sprintf("{\"event\":\"Signed Up\",\"properties\":{\"Referred By\":\"Friend\",\"distinct_id\":\"13793\",\"time\":%d,\"token\":\"e3bc4100330c35722740fb8c6f5abddc\"}}", importTime.Unix())

	if !reflect.DeepEqual(decodeBody(), want) {
		t.Errorf("LastRequest.URL returned %+v, want %+v",
			decodeBody(), want)
	}

	want = "/import"
	path := LastRequest.URL.Path

	if !reflect.DeepEqual(path, want) {
		t.Errorf("path returned %+v, want %+v",
			path, want)
	}
}

func TestGroupOperations(t *testing.T) {
	setup()
	defer teardown()

	client.UpdateGroup(context.TODO(), "company_id", "11", &Update{
		Operation: "$set",
		Properties: map[string]interface{}{
			"Address":  "1313 Mockingbird Lane",
			"Birthday": "1948-01-01",
		},
	})

	want := "{\"$group_id\":\"11\",\"$group_key\":\"company_id\",\"$set\":{\"Address\":\"1313 Mockingbird Lane\",\"Birthday\":\"1948-01-01\"},\"$token\":\"e3bc4100330c35722740fb8c6f5abddc\"}"

	if !reflect.DeepEqual(decodeBody(), want) {
		t.Errorf("LastRequest.URL returned %+v, want %+v",
			decodeBody(), want)
	}

	want = "/groups"
	path := LastRequest.URL.Path

	if !reflect.DeepEqual(path, want) {
		t.Errorf("path returned %+v, want %+v",
			path, want)
	}
}

func TestUpdate(t *testing.T) {
	setup()
	defer teardown()

	client.Update(context.TODO(), "13793", &Update{
		Operation: "$set",
		Properties: map[string]interface{}{
			"Address":  "1313 Mockingbird Lane",
			"Birthday": "1948-01-01",
		},
	})

	want := "{\"$distinct_id\":\"13793\",\"$set\":{\"Address\":\"1313 Mockingbird Lane\",\"Birthday\":\"1948-01-01\"},\"$token\":\"e3bc4100330c35722740fb8c6f5abddc\"}"

	if !reflect.DeepEqual(decodeBody(), want) {
		t.Errorf("LastRequest.URL returned %+v, want %+v",
			decodeBody(), want)
	}

	want = "/engage"
	path := LastRequest.URL.Path

	if !reflect.DeepEqual(path, want) {
		t.Errorf("path returned %+v, want %+v",
			path, want)
	}
}

func TestError(t *testing.T) {
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"error": "some error", "status": "0"}`))
		LastRequest = r
	}))

	assertErrTrackFailed := func(err error) {
		merr, ok := err.(*MixpanelError)

		if !ok {
			t.Errorf("Error should be wrapped in a MixpanelError: %v", err)
			return
		}

		terr, ok := merr.Err.(*ErrTrackFailed)

		if !ok {
			t.Errorf("Error should be a *ErrTrackFailed: %v", err)
			return
		}

		if terr.Message != "error=some error; status=0; httpCode=200" &&
			terr.Message != "error=some error; status=0; httpCode=200, body={\"error\": \"some error\", \"status\": \"0\"}" {
			t.Errorf("Wrong body carried in the *ErrTrackFailed: %q", terr.Message)
		}
	}

	client = New("e3bc4100330c35722740fb8c6f5abddc", ts.URL)

	assertErrTrackFailed(client.Update(context.TODO(), "1", &Update{}))
	assertErrTrackFailed(client.Track(context.TODO(), "1", "name", &Event{}))
	assertErrTrackFailed(client.Import(context.TODO(), "1", "name", &Event{}))
}

func TestUnwrapCompatible(t *testing.T) {
	mErr := &MixpanelError{Err: context.DeadlineExceeded}
	err := error(mErr)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("not compatible with unwrap")
	}
}
