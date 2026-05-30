package common

import (
"bytes"
"encoding/json"
"io"
"net/http"
"time"
)

func HttpGet(url string) []byte {
client := &http.Client{Timeout: 15 * time.Second}
for i := 0; i < 3; i++ {
resp, err := client.Get(url)
if err == nil {
defer resp.Body.Close()
b, _ := io.ReadAll(resp.Body)
return b
}
time.Sleep(1 * time.Second)
}
return nil
}

func HttpPost(url string, v interface{}) []byte {
client := &http.Client{Timeout: 15 * time.Second}
body, _ := json.Marshal(v)
for i := 0; i < 3; i++ {
resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
if err == nil {
defer resp.Body.Close()
b, _ := io.ReadAll(resp.Body)
return b
}
time.Sleep(1 * time.Second)
}
return nil
}
