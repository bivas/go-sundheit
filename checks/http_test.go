package checks

import (
	"testing"
	"net/http/httptest"
	"net/http"
	"strings"
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	longRequest     = "LongRequest"
	expectedContent = "I'm healthy"
)

func TestNewHttpCheckRequiredFields(t *testing.T) {
	check, err := NewHTTPCheck(nil)
	assert.Nil(t, check, "nil config should yield nil check")
	assert.Error(t, err, "nil config should yield error")

	check, err = NewHTTPCheck(&HTTPCheckConfig{
		CheckName: "meh",
	})
	assert.Nil(t, check, "nil URL should yield nil check")
	assert.Error(t, err, "nil URL should yield error")

	check, err = NewHTTPCheck(&HTTPCheckConfig{
		URL: "http://example.org",
	})
	assert.Nil(t, check, "nil CheckName should yield nil check")
	assert.Error(t, err, "nil CheckName should yield error")

	check, err = NewHTTPCheck(&HTTPCheckConfig{
		URL:       ":/invalid.url",
		CheckName: "invalid.url",
	})
	assert.Nil(t, check, "invalid url should yield nil check")
	assert.Error(t, err, "invalid url should yield error")
}

func TestNewHttpCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.URL.String(), longRequest) {
			waitDuration, err := time.ParseDuration(req.URL.Query().Get("wait"))
			if err != nil {
				fmt.Println("Failed to parse sleep duration: ", err)
				waitDuration = 10 * time.Second
			}

			time.Sleep(waitDuration)
		}

		rw.WriteHeader(200)
		rw.Write([]byte(expectedContent))
	}))

	defer server.Close()

	t.Run("HttpCheck success call", testHTTPCheckSuccess(server.URL, server.Client()))
	t.Run("HttpCheck success call with body check", testHTTPCheckSuccessWithExpectedBody(server.URL, server.Client()))
	t.Run("HttpCheck success call with failing body check", testHTTPCheckFailWithUnexpectedBody(server.URL, server.Client()))
	t.Run("HttpCheck fail on status code", testHTTPCheckFailStatusCode(server.URL, server.Client()))
	t.Run("HttpCheck fail on URL", testHTTPCheckFailURL(server.URL, server.Client()))
	t.Run("HttpCheck fail on timeout", testHTTPCheckFailTimeout(server.URL, server.Client()))
}

func testHTTPCheckSuccess(url string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName: "url.check",
			URL:       url,
			Client:    client,
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Nil(t, err, "check should pass")
		assert.Equal(t, fmt.Sprintf("URL [%s] is accessible", url), details, "check should pass")
	}
}

func testHTTPCheckSuccessWithExpectedBody(url string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName:    "url.check",
			URL:          url,
			Client:       client,
			ExpectedBody: expectedContent,
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Nil(t, err, "check should pass")
		assert.Equal(t, fmt.Sprintf("URL [%s] is accessible", url), details, "check should pass")
	}
}

func testHTTPCheckFailWithUnexpectedBody(url string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName:    "url.check",
			URL:          url,
			Client:       client,
			ExpectedBody: "my body is a temple",
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Error(t, err, "check should fail")
		assert.Equal(t, "body does not contain expected content 'my body is a temple'", err.Error(), "check error message")
		assert.Equal(t, url, details, "check details when fail are the URL")
	}
}

func testHTTPCheckFailStatusCode(url string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName:      "url.check",
			URL:            url,
			Client:         client,
			ExpectedStatus: 300,
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Error(t, err, "check should fail")
		assert.Equal(t, "unexpected status code: '200' expected: '300'", err.Error(), "check error message")
		assert.Equal(t, url, details, "check details when fail are the URL")
	}
}

func testHTTPCheckFailURL(_ string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		bogusURL := "http://devil.dot.com:666"
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName: "url.check",
			URL:       bogusURL,
			Client:    client,
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Error(t, err, "check should fail")
		assert.Contains(t, err.Error(), "no such host", "check error message")
		assert.Equal(t, bogusURL, details, "check details when fail are the URL")
	}
}

func testHTTPCheckFailTimeout(url string, client *http.Client) func(t *testing.T) {
	return func(t *testing.T) {
		waitURL := fmt.Sprintf("%s/%s?wait=%s", url, longRequest, 100*time.Millisecond)
		check, err := NewHTTPCheck(&HTTPCheckConfig{
			CheckName: "url.check",
			URL:       waitURL,
			Client:    client,
			Timeout:   10 * time.Millisecond,
		})
		assert.Nil(t, err)

		details, err := check.Execute()
		assert.Error(t, err, "check should fail")
		assert.Contains(t, err.Error(), "Client.Timeout exceeded", "check error message")
		assert.Equal(t, waitURL, details, "check details when fail are the URL")
	}
}
