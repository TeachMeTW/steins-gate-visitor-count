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

    "github.com/nfnt/resize"
)

// cacheImages loads images into memory
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

// generateMd5 creates an MD5 hash of the provided string
func generateMd5(id string) (string, error) {
    w := md5.New()
    if _, err := io.WriteString(w, id); err != nil {
        return "", err
    }
    res := fmt.Sprintf("%x", w.Sum(nil))
    return res, nil
}

// updateCounter increments the visit count using CounterAPI
func updateCounter(key string) string {
    namespace := "github-visitor-counter"
    name := "teachmetw-visit"

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

// generateImage creates an image from the count
func generateImage(digits []image.Image, count string) image.Image {
    length := len(count)
    img := image.NewNRGBA(image.Rect(0, 0, 200*length, 200))
    for i := range count {
        index, _ := strconv.Atoi(count[i : i+1])
        draw.Draw(img, image.Rect(i*200, 0, 200*(i+1), 200), digits[index], digits[index].Bounds().Min, draw.Over)
    }
    return img
}

// resizeImage resizes an image by a given ratio
func resizeImage(img image.Image, ratio float64) image.Image {
    width := uint(float64(img.Bounds().Dx()) * ratio)
    height := uint(float64(img.Bounds().Dy()) * ratio)
    return resize.Resize(width, height, img, resize.Lanczos3)
}

// Handler is the exported function that Vercel will invoke
func Handler(w http.ResponseWriter, r *http.Request) {
    digits := make([]image.Image, 10)
    cacheImages(&digits)

    // Extract the ID from the URL path
    id := r.URL.Path[len("/"):]
    if id == "" {
        id = "default" // Provide a default ID if none is specified
    }

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

    // Set headers to avoid caching
    expireTime := time.Now().Add(-10 * time.Minute).Format(http.TimeFormat)
    w.Header().Set("Expires", expireTime)
    w.Header().Set("Cache-Control", "no-cache, max-age=0, no-store, s-maxage=0, proxy-revalidate")

    // Send the image as a response
    w.Header().Set("Content-Type", "image/png")
    w.WriteHeader(http.StatusOK)
    w.Write(buf.Bytes())
}
