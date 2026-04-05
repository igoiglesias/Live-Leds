package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type LED struct {
	Name       string
	Brightness uint8
}

const ledsPath = "/sys/class/leds"

func ListLEDs() []LED {
	entries, err := os.ReadDir(ledsPath)
	if err != nil {
		fmt.Errorf("failed to read LEDs directory: %w", err)
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
	var leds []LED

	http.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
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
				<h1>LEDs</h2>
			</div>
			<div>
				<form method="POST" action="/leds/set">
					<p>Selecione os LEDs que deseja encender:</p>
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
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		
		selectedLEDs := r.Form["led"]
		for _, led := range leds {
			brightness := uint8(0)
			for _, selected := range selectedLEDs {
				if selected == led.Name {
					brightness = 255
					break
				}
			}
			if err := SetLedBrightness(led.Name, brightness); err != nil {
				fmt.Printf("Failed to set brightness for %s: %v\n", led.Name, err)
			}			
		}
		http.Redirect(w, r, "/leds", http.StatusSeeOther)
	})

	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)

}