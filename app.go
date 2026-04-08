package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = make([]byte, 32)

func initJWTKey() {
	_, err := rand.Read(jwtSecret)
	if err != nil {
		panic(err)
	}
}

type CustomClaims struct {
	jwt.RegisteredClaims
}

func CreateToken() (string, error) {
	claims := CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "led-control",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ValidateToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&CustomClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, fmt.Errorf("invalid token")
	}
}

func SetCookie(w http.ResponseWriter) {
	tokenString, _ := CreateToken()
	http.SetCookie(w, &http.Cookie{
		Name: "anti_ryan",
		Value: tokenString,
		Path: "/",
		Domain: "leds.igoriglesias.com",
		HttpOnly: true,
		// Expires: time.Now().Add(10 * time.Minute),
		Secure: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func VerifyCookie(r *http.Request) error {
	csrfToken, err := r.Cookie("anti_ryan")

	if err != nil {
		return fmt.Errorf("Missing CSRF token")
	}

	if _, err := ValidateToken(csrfToken.Value); err != nil {
		return fmt.Errorf("Invalid CSRF token")
	}

	return nil
}


type LED struct {
	Name       string
	Brightness uint8
}

const ledsPath = "/sys/class/leds"

func ListLEDs() []LED {
	entries, err := os.ReadDir(ledsPath)
	if err != nil {
		fmt.Printf("failed to read LEDs directory: %s", err)
	}

	var leds []LED
	for _, e := range entries {
		path := filepath.Join(ledsPath, e.Name(), "brightness")
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("Failed to read brightness for %s: %v\n", e.Name(), err)
			continue
		}
		cleanData := strings.ReplaceAll(string(data), "\n", "")
		brightness, err := strconv.Atoi(cleanData)
		if err != nil {
			fmt.Printf("Failed to parse brightness for %s: %v\n", e.Name(), err)
			continue
		}
		leds = append(leds, LED{Name: e.Name(), Brightness: uint8(brightness)})
	}
	return leds
}

func SetLedBrightness(ledName string, brightness uint8) error {
	path := filepath.Join(ledsPath, ledName, "brightness")
	return os.WriteFile(path, []byte(strconv.Itoa(int(brightness))), 0644)
}

func main(){
	initJWTKey()
	var leds []LED

	http.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		SetCookie(w)

		leds = ListLEDs()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>LED Control</title>
			<link rel="stylesheet" href="https://cdn.simplecss.org/simple.min.css">
		</head>
		<body>
			<div>
				<h1>LEDs</h1>
				<small>Esse é um projeto de automação feito em <a href="https://www.youtube.com/watch?v=L0cuSrDs8So&list=PLwuyavmn5qKrxLaiUoGfRgkB-7uwYNx8o">Live no YouTube</a>.<br>Ele consiste em um roteador Tp-Link WR740N com Firmware OpenWrt e um aplicação em GO rodando internamente e tomando o controle dos LEDs. Dessa forma, através de uma interface web, é possível controlar o brilho dos LEDs.</small>
			</div>
			<div>
				<form method="POST" action="/leds/set">
					<p>Selecione os LEDs que deseja acender:</p>
					<p>
					` + func() string {
						var items string
						for i, led := range leds[1:] {
							ledID := i + 1
							checked := ""
							title := strings.Split(led.Name, ":")[2]
							if led.Brightness > 0 {
								checked = "checked"
							}	
							items += fmt.Sprintf("<input type='checkbox' id='led-%d' name='led' value='%s' %s>", ledID, led.Name, checked)
							items += fmt.Sprintf("<label for='led-%d' title='LED %s'>%s</label>", ledID, title, title)
						}
						return items
					}() + `
					</p>
					<br>
					<br>
					<input type="submit" value="Enviar">
				</form>
			<div>
		</body>
		</html>
		`))
	})

	http.HandleFunc("POST /leds/set", func(w http.ResponseWriter, r *http.Request) {
		err := VerifyCookie(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		
		selectedLEDs := r.Form["led"]
		for _, led := range leds {
			brightness := uint8(0)
			
			if slices.Contains(selectedLEDs, led.Name){
				brightness = 255
			}
			
			if slices.Contains(selectedLEDs, led.Name) {
				brightness = 255
			}

			if brightness == led.Brightness	{
				continue
			}

			if err := SetLedBrightness(led.Name, brightness); err != nil {
				fmt.Printf("Failed to set brightness for %s: %v\n", led.Name, err)
			}			
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)

}