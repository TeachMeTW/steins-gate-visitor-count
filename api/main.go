package handler

import (
    "bytes"
    "crypto/md5"
    "embed"
    "encoding/json"
    "fmt"
    "image"
    "image/draw"
    "image/png"
    "io"
    "log"
    "net/http"
    "strconv"
    "time"

    "github.com/nfnt/resize"
)

//go:embed digits/*.png
var digitImages embed.FS

// cacheImages loads images into memory
func cacheImages() ([]image.Image, error) {
    digits := make([]image.Image, 10)
    for i := 0; i < 10; i++ {
        fileName := fmt.Sprintf("digits/%d.png", i)
        fileData, err := digitImages.Open(fileName)
        if err != nil {
            return nil, fmt.Errorf("failed to open image %s: %v", fileName, err)
        }
        defer fileData.Close()

        img, _, err := image.Decode(fileData)
        if err != nil {
            return nil, fmt.Errorf("failed to decode image %s: %v", fileName, err)
        }
        digits[i] = img
    }
    return digits, nil
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
func updateCounter(key string) (string, error) {
    namespace := "github-visitor-counter"
    name := key // Use the key as the name to make the counter ID-specific

    url := fmt.Sprintf("https://api.counterapi.dev/v1/%s/%s/up/", namespace, name)
    resp, err := http.Get(url)
    if err != nil {
        log.Println("Error fetching counter:", err)
        return "", err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Println("Non-OK HTTP status:", resp.StatusCode)
        return "", fmt.Errorf("non-OK HTTP status: %d", resp.StatusCode)
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Println("Error reading response body:", err)
        return "", err
    }

    // Parse the JSON response to extract the counter value
    var result struct {
        Value int `json:"value"`
    }
    if err := json.Unmarshal(body, &result); err != nil {
        log.Println("Error parsing JSON:", err)
        return "", err
    }

    return strconv.Itoa(result.Value), nil
}

// generateImage creates an image from the count
func generateImage(digits []image.Image, count string) (image.Image, error) {
    length := len(count)
    img := image.NewNRGBA(image.Rect(0, 0, 200*length, 200))
    for i, c := range count {
        index, err := strconv.Atoi(string(c))
        if err != nil || index < 0 || index > 9 {
            return nil, fmt.Errorf("invalid digit '%c' in count", c)
        }
        draw.Draw(img, image.Rect(i*200, 0, 200*(i+1), 200), digits[index], digits[index].Bounds().Min, draw.Over)
    }
    return img, nil
}

// resizeImage resizes an image by a given ratio
func resizeImage(img image.Image, ratio float64) image.Image {
    width := uint(float64(img.Bounds().Dx()) * ratio)
    height := uint(float64(img.Bounds().Dy()) * ratio)
    return resize.Resize(width, height, img, resize.Lanczos3)
}

// Handler is the exported function that Vercel will invoke
func Handler(w http.ResponseWriter, r *http.Request) {
    digits, err := cacheImages()
    if err != nil {
        log.Println("Error loading images:", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Extract the ID from the URL path
    id := r.URL.Path[len("/"):]
    if id == "" {
        id = "default" // Provide a default ID if none is specified
    }

    m, err := generateMd5(id)
    if err != nil {
        log.Println("Error generating MD5:", err)
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }

    count, err := updateCounter(m)
    if err != nil {
        log.Println("Fetch visitor count error:", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Generate image with the count
    img, err := generateImage(digits, count)
    if err != nil {
        log.Println("Error generating image:", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

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
        log.Println("Error encoding PNG:", err)
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
