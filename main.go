package main

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

func main() {
	port := os.Getenv("port")
	if port == "" {
		port = "8080"
	}

	digits := make([]image.Image, 10)
	cacheImages(&digits)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	e.GET("/:id", func(c echo.Context) error {
		id := c.Param("id")

		m, err := generateMd5(id)
		if err != nil {
			log.Println(err)
			return c.NoContent(http.StatusBadRequest)
		}

		count := updateCounter(m)
		if count == "" {
			log.Println("Fetch visitor count error.")
			return c.NoContent(http.StatusInternalServerError)
		}

		img := generateImage(digits, count)
		if v := c.QueryParam("ratio"); len(v) != 0 {
			if ratio, err := strconv.ParseFloat(v, 64); err == nil && ratio > 0 && ratio <= 2 {
				img = resizeImage(img, ratio)
			}
		}
		buf := new(bytes.Buffer)
		err = png.Encode(buf, img)
		if err != nil {
			log.Println(err)
			return c.NoContent(http.StatusInternalServerError)
		}

		expireTime := time.Now().Add(-10 * time.Minute).String()
		c.Response().Header().Add("Expires", expireTime)
		c.Response().Header().Add("Cache-Control", "no-cache,max-age=0,no-store,s-maxage=0,proxy-revalidate")

		return c.Blob(http.StatusOK, "image/png", buf.Bytes())
	})

	e.Logger.Fatal(e.Start(":" + port))
}
