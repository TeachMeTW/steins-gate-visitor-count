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
    "sync"
    "time"

    "github.com/nfnt/resize"
)

//go:embed digits/*.png
var digitImages embed.FS

var (
    digits    []image.Image
    cacheOnce sync.Once
)

// cacheImages loads images into memory
func cacheImages() ([]image.Image, error) {
    digits := make([]image.Image, 10)
    for i := 0; i < 10; i++ {
        fileName := fmt.Sprintf("digits/%d.png", i)
        fileData, err := digitImages.Open(fileName)
        if err != nil {
            return nil, fmt.Errorf("failed to open image %s: %v", fileName, err)
        }

        img, _, err := image.Decode(fileData)
        fileData.Close() // Close the file after decoding
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

func updateCounter(counterName string) (string, error) {
    // Replace with your Firebase Realtime Database URL
    firebaseDatabaseURL := "https://teachmetw-counter-default-rtdb.firebaseio.com/"

    // Path to the counter in the database
    counterPath := fmt.Sprintf("/counters/%s/count.json", counterName)

    // The URL to increment the counter using a transaction
    url := firebaseDatabaseURL + counterPath

    // Create a custom HTTP client with a timeout
    client := &http.Client{
        Timeout: 5 * time.Second,
    }

    // The transaction payload to increment the counter
    payload := []byte(`{"count": {".sv": {"increment": 1}}}`)

    // Create a PATCH request to increment the counter atomically
    req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(payload))
    if err != nil {
        log.Println("Error creating request:", err)
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")

    // Send the request
    resp, err := client.Do(req)
    if err != nil {
        log.Println("Error updating counter:", err)
        return "", err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Println("Non-OK HTTP status:", resp.StatusCode)
        bodyBytes, _ := io.ReadAll(resp.Body)
        log.Println("Response body:", string(bodyBytes))
        return "", fmt.Errorf("non-OK HTTP status: %d", resp.StatusCode)
    }

    // Retrieve the updated counter value from the response
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Println("Error reading response body:", err)
        return "", err
    }

    // Parse the JSON response to extract the counter value
    var result map[string]int
    if err := json.Unmarshal(body, &result); err != nil {
        log.Println("Error parsing JSON:", err)
        return "", err
    }

    count := result["count"]
    return strconv.Itoa(count), nil
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
    // Load images only once
    cacheOnce.Do(func() {
        var err error
        digits, err = cacheImages()
        if err != nil {
            log.Println("Error loading images:", err)
            digits = nil
        }
    })

    if digits == nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Use 'teachmetw' as the counter name
    id := "teachmetw"

    // Fetch and increment the counter
    count, err := updateCounter(id)
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

// // Temporary main function for local testing
// func main() {
//     port := ":8080"

//     http.HandleFunc("/", Handler)

//     log.Printf("Starting server on http://localhost%s\n", port)
//     if err := http.ListenAndServe(port, nil); err != nil {
//         log.Fatalf("Failed to start server: %v", err)
//     }
// }
