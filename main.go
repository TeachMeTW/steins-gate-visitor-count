package handler

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nfnt/resize"
)

func cacheImages(digits *[]image.Image) {
	cacheOneImage := func(no int) {
		file, _ := os.Open(fmt.Sprintf("digits/%d.png", no))
		defer file.Close()
		(*digits)[no], _, _ = image.Decode(file)
	}
	for i := range *digits {
		cacheOneImage(i)
	}
}

func generateMd5(id string) (string, error) {
	w := md5.New()
	if _, err := io.WriteString(w, id); err != nil {
		return "", err
	}
	res := fmt.Sprintf("%x", w.Sum(nil))
	return res, nil
}

// UpdateCounter now uses CounterAPI to track GitHub visits
func updateCounter(key string) string {
	namespace := "github-visitor-counter"
	name := "teachmetw-visit"

	// Use CounterAPI to count up
	url := fmt.Sprintf("https://api.counterapi.dev/v1/%s/%s/up", namespace, name)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching counter:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Non-OK HTTP status:", resp.StatusCode)
		return ""
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return ""
	}

	return string(body)
}

func generateImage(digits []image.Image, count string) image.Image {
	length := len(count)
	img := image.NewNRGBA(image.Rect(0, 0, 200*length, 200))
	for i := range count {
		index, _ := strconv.Atoi(count[i : i+1])
		draw.Draw(img, image.Rect(i*200, 0, 200*length, 200), digits[index], digits[index].Bounds().Min, draw.Over)
	}
	return img
}

// resizeImage resize image to specified ratio
func resizeImage(img image.Image, ratio float64) image.Image {
	width := uint(float64(img.Bounds().Max.X-img.Bounds().Min.X) * ratio)
	height := uint(float64(img.Bounds().Max.Y-img.Bounds().Min.Y) * ratio)
	return resize.Resize(width, height, img, resize.Lanczos3)
}

// Exported HTTP handler for Vercel
func Handler(w http.ResponseWriter, r *http.Request) {
	digits := make([]image.Image, 10)
	cacheImages(&digits)

	// Extract the ID from the URL
	id := r.URL.Path[len("/"):]

	m, err := generateMd5(id)
	if err != nil {
		log.Println(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	count := updateCounter(m)
	if count == "" {
		log.Println("Fetch visitor count error.")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Generate image with the count
	img := generateImage(digits, count)

	// Check for the 'ratio' query parameter
	ratioStr := r.URL.Query().Get("ratio")
	if len(ratioStr) != 0 {
		if ratio, err := strconv.ParseFloat(ratioStr, 64); err == nil && ratio > 0 && ratio <= 2 {
			img = resizeImage(img, ratio)
		}
	}

	buf := new(bytes.Buffer)
	err = png.Encode(buf, img)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set headers
	expireTime := time.Now().Add(-10 * time.Minute).String()
	w.Header().Set("Expires", expireTime)
	w.Header().Set("Cache-Control", "no-cache,max-age=0,no-store,s-maxage=0,proxy-revalidate")

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
