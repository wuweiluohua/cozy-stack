package settings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const domain = "cozysettings.example.net"

var ts *httptest.Server
var testInstance *instance.Instance
var instanceRev string

func TestThemeCSS(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/theme.css")
	assert.NoError(t, err)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, []byte(":root"), body[:5])
}

func TestDiskUsage(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/disk-usage")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/disk-usage", nil)
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.settings", data["type"].(string))
	assert.Equal(t, "io.cozy.settings.disk-usage", data["id"].(string))
	attrs, ok := data["attributes"].(map[string]interface{})
	assert.True(t, ok)
	used, ok := attrs["used"].(string)
	assert.True(t, ok)
	assert.Equal(t, "0", used)
}

func TestRegisterPassphraseWrongToken(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "BADBEEF",
	})
	res1, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	args, _ = json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": "XYZ",
	})
	res2, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
}

func TestRegisterPassphraseCorrectToken(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"passphrase":     "MyFirstPassphrase",
		"register_token": hex.EncodeToString(testInstance.RegisterToken),
	})
	res, err := http.Post(ts.URL+"/settings/passphrase", "application/json", bytes.NewReader(args))
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestUpdatePassphraseWithWrongPassphrase(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "BADBEEF",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "400 Bad Request", res.Status)
}

func TestUpdatePassphraseSuccess(t *testing.T) {
	args, _ := json.Marshal(&echo.Map{
		"new_passphrase":     "MyPassphrase",
		"current_passphrase": "MyFirstPassphrase",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/passphrase", bytes.NewReader(args))
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "204 No Content", res.Status)
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestGetInstance(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/instance")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	testInstance.RegisterToken = []byte("test")
	res, err = http.Get(ts.URL + "/settings/instance?registerToken=test")
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	testInstance.RegisterToken = []byte{}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/instance", nil)
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.settings", data["type"].(string))
	assert.Equal(t, "io.cozy.settings.instance", data["id"].(string))
	meta, ok := data["meta"].(map[string]interface{})
	assert.True(t, ok)
	instanceRev = meta["rev"].(string)
	assert.NotEmpty(t, instanceRev)
	attrs, ok := data["attributes"].(map[string]interface{})
	assert.True(t, ok)
	email, ok := attrs["email"].(string)
	assert.True(t, ok)
	assert.Equal(t, "alice@example.com", email)
	tz, ok := attrs["tz"].(string)
	assert.True(t, ok)
	assert.Equal(t, "Europe/Berlin", tz)
	locale, ok := attrs["locale"].(string)
	assert.True(t, ok)
	assert.Equal(t, "en", locale)
}

func TestUpdateInstance(t *testing.T) {
	checkResult := func(res *http.Response) {
		assert.Equal(t, 200, res.StatusCode)
		var result map[string]interface{}
		err := json.NewDecoder(res.Body).Decode(&result)
		assert.NoError(t, err)
		data, ok := result["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "io.cozy.settings", data["type"].(string))
		assert.Equal(t, "io.cozy.settings.instance", data["id"].(string))
		meta, ok := data["meta"].(map[string]interface{})
		assert.True(t, ok)
		rev := meta["rev"].(string)
		assert.NotEmpty(t, rev)
		assert.NotEqual(t, instanceRev, rev)
		attrs, ok := data["attributes"].(map[string]interface{})
		assert.True(t, ok)
		email, ok := attrs["email"].(string)
		assert.True(t, ok)
		assert.Equal(t, "alice@example.org", email)
		tz, ok := attrs["tz"].(string)
		assert.True(t, ok)
		assert.Equal(t, "Europe/London", tz)
		locale, ok := attrs["locale"].(string)
		assert.True(t, ok)
		assert.Equal(t, "fr", locale)
	}

	body := `{
		"data": {
			"type": "io.cozy.settings",
			"id": "io.cozy.settings.instance",
			"meta": {
				"rev": "%s"
			},
			"attributes": {
				"tz": "Europe/London",
				"email": "alice@example.org",
				"locale": "fr"
			}
		}
	}`
	body = fmt.Sprintf(body, instanceRev)
	req, _ := http.NewRequest("PUT", ts.URL+"/settings/instance", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	checkResult(res)

	req, _ = http.NewRequest("GET", ts.URL+"/settings/instance", nil)
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	checkResult(res)
}

func TestListClients(t *testing.T) {
	res, err := http.Get(ts.URL + "/settings/clients")
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)

	client := &oauth.Client{
		RedirectURIs:    []string{"http:/localhost:4000/oauth/callback"},
		ClientName:      "Cozy-desktop on my-new-laptop",
		ClientKind:      "desktop",
		ClientURI:       "https://docs.cozy.io/en/mobile/desktop.html",
		LogoURI:         "https://docs.cozy.io/assets/images/cozy-logo-docs.svg",
		PolicyURI:       "https://cozy.io/policy",
		SoftwareID:      "/github.com/cozy-labs/cozy-desktop",
		SoftwareVersion: "0.16.0",
	}
	regErr := client.Create(testInstance)
	assert.Nil(t, regErr)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/settings/clients", nil)
	req.Header.Add("Authorization", "Bearer "+testClientsToken(testInstance))
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data := result["data"].([]interface{})
	assert.Len(t, data, 1)
	obj := data[0].(map[string]interface{})
	assert.Equal(t, "io.cozy.oauth.clients", obj["type"].(string))
	assert.Equal(t, client.ClientID, obj["id"].(string))
	links := obj["links"].(map[string]interface{})
	self := links["self"].(string)
	assert.Equal(t, "/settings/clients/"+client.ClientID, self)
	attrs := obj["attributes"].(map[string]interface{})
	redirectURIs := attrs["redirect_uris"].([]interface{})
	assert.Len(t, redirectURIs, 1)
	assert.Equal(t, client.RedirectURIs[0], redirectURIs[0].(string))
	assert.Equal(t, client.ClientName, attrs["client_name"].(string))
	assert.Equal(t, client.ClientKind, attrs["client_kind"].(string))
	assert.Equal(t, client.ClientURI, attrs["client_uri"].(string))
	assert.Equal(t, client.LogoURI, attrs["logo_uri"].(string))
	assert.Equal(t, client.PolicyURI, attrs["policy_uri"].(string))
	assert.Equal(t, client.SoftwareID, attrs["software_id"].(string))
	assert.Equal(t, client.SoftwareVersion, attrs["software_version"].(string))
	assert.Nil(t, attrs["client_secret"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	instance.Destroy(domain)
	testInstance, _ = instance.Create(&instance.Options{
		Domain:   domain,
		Locale:   "en",
		Timezone: "Europe/Berlin",
		Email:    "alice@example.com",
	})

	r := echo.New()
	r.HTTPErrorHandler = errors.ErrorHandler
	Routes(r.Group("/settings", injectInstance(testInstance)))

	ts = httptest.NewServer(r)
	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.Exit(res)
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}

func testToken(i *instance.Instance) string {
	t, _ := crypto.NewJWT(testInstance.OAuthSecret, permissions.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: permissions.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  "testapp",
		},
		Scope: consts.Settings,
	})
	return t
}

func testClientsToken(i *instance.Instance) string {
	t, _ := crypto.NewJWT(testInstance.OAuthSecret, permissions.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: permissions.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  "testapp",
		},
		Scope: consts.OAuthClients,
	})
	return t
}
