package platform

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCiscoLoginCopiesHeaderSessionIntoCookies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})

	for _, test := range []struct {
		code       string
		userStatus string
	}{{"0", "ok"}, {"10", "simple"}, {"12", "initial"}, {"13", "dueExpire"}, {"14", "initComp"}} {
		t.Run(test.code, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					http.Redirect(w, r, "/test/mts/config/log_off_page.htm", http.StatusFound)
				case "/test/mts/config/log_off_page.htm":
					_, _ = w.Write([]byte("login"))
				case "/test/mts/config/device/wcd":
					_, _ = fmt.Fprintf(w, `<ResponseData><DeviceConfiguration><EncryptionSetting><passwEncryptEnable>1</passwEncryptEnable><rsaPublicKey>%s</rsaPublicKey><loginToken>test-token</loginToken></EncryptionSetting></DeviceConfiguration></ResponseData>`, publicKey)
				case "/test/mts/config/system.xml":
					w.Header().Set("sessionID", "test-session")
					_, _ = fmt.Fprintf(w, `<ResponseData><ActionStatus><statusCode>%s</statusCode><statusString>accepted</statusString></ActionStatus></ResponseData>`, test.code)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			services, err := NewSG350WebServices(SG350WebConfig{Address: server.URL, Username: "admin", Password: "test-password", AllowInsecureTLS: true})
			if err != nil {
				t.Fatal(err)
			}
			adapter := services.Telemetry.(*SG350WebAdapter)
			baseURL, _ := url.Parse(server.URL)
			cookies := map[string]string{}
			for _, cookie := range adapter.client.Jar.Cookies(baseURL) {
				cookies[cookie.Name] = cookie.Value
			}
			if cookies["sessionID"] != "test-session" || cookies["userStatus"] != test.userStatus || cookies["app"] != "switch" {
				t.Fatalf("cookies = %#v", cookies)
			}
		})
	}
}
