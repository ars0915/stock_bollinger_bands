// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ars0915/stock_bollinger_bands/bollinger_bands"
)

var clientID string
var clientSecret string
var callbackURL string
var tokens []string

func main() {
	http.HandleFunc("/", healthHandler)
	http.HandleFunc("/callback", callbackHandler)
	http.HandleFunc("/notify", notifyHandler)
	http.HandleFunc("/auth", authHandler)
	http.HandleFunc("/stock", stockHandler)
	http.HandleFunc("/getTokens", getTokenHandler)
	http.HandleFunc("/setTokens", setTokenHandler)
	clientID = os.Getenv("ClientID")
	clientSecret = os.Getenv("ClientSecret")
	callbackURL = os.Getenv("CallbackURL")
	port := os.Getenv("ServePort")
	xd := os.Getenv("PORT")
	fmt.Println(xd)
	if port == "" {
		port = "8080"
	}

	fmt.Printf("ENV port:%s, cid:%s csecret:%s\n", port, clientID, clientSecret)
	addr := fmt.Sprintf(":%s", port)
	http.ListenAndServe(addr, nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {

}

func getTokenHandler(w http.ResponseWriter, r *http.Request) {
	t := strings.Join(tokens, ",")
	w.Write([]byte(t))
}

func setTokenHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	t := r.Form.Get("tokens")
	fmt.Printf("Get tokens=%s\n", t)

	tokens = strings.Split(t, ",")
}

func stockHandler(w http.ResponseWriter, r *http.Request) {
	targets := bollinger_bands.Run()

	data := url.Values{}
	data.Add("message", fmt.Sprintf("%v", targets))

	var result []byte
	for _, token := range tokens {
		byt, err := apiCall("POST", apiNotify, data, token)
		fmt.Println("ret:", string(byt), " err:", err)

		res := newTokenResponse(byt)
		fmt.Println("result:", res)
		result = append(result, byt...)
	}
	w.Write(result)
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm() // Populates request.Form
	msg := r.Form.Get("msg")
	fmt.Printf("Get msg=%s\n", msg)

	data := url.Values{}
	data.Add("message", msg)

	var result []byte
	for _, token := range tokens {
		byt, err := apiCall("POST", apiNotify, data, token)
		fmt.Println("ret:", string(byt), " err:", err)

		res := newTokenResponse(byt)
		fmt.Println("result:", res)
		result = append(result, byt...)
	}
	w.Write(result)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm() // Populates request.Form
	code := r.Form.Get("code")
	state := r.Form.Get("state")
	fmt.Printf("Get code=%s, state=%s \n", code, state)

	data := url.Values{}
	data.Add("grant_type", "authorization_code")
	data.Add("code", code)
	data.Add("redirect_uri", callbackURL)
	data.Add("client_id", clientID)
	data.Add("client_secret", clientSecret)

	byt, err := apiCall("POST", apiToken, data, "")
	fmt.Println("ret:", string(byt), " err:", err)

	res := newTokenResponse(byt)
	fmt.Println("result:", res)
	tokens = append(tokens, res.AccessToken)
	w.Write(byt)
}
func authHandler(w http.ResponseWriter, r *http.Request) {
	check := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	t, err := template.New("webpage").Parse(authTmpl)
	check(err)
	noItems := struct {
		ClientID    string
		CallbackURL string
	}{
		ClientID:    clientID,
		CallbackURL: callbackURL,
	}

	err = t.Execute(w, noItems)
	check(err)
}
