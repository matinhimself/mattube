package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/matinhimself/mattube/client/fronting"
)

// TestFronting runs 4 connectivity checks against the given fronting IP and SNI,
// porting the logic from telegram-crawler/tools/test_fronting.py.
func TestFronting(frontingIP, allowedSNI string) {
	if frontingIP == "" || allowedSNI == "" {
		fatalf("usage: test-fronting <fronting-ip> <allowed-sni>\nexample: test-fronting 216.239.38.120 www.google.com")
	}

	client := fronting.NewClient(frontingIP, allowedSNI)
	client.Timeout = 15 * time.Second

	pass, fail := 0, 0
	run := func(name string, fn func() error) {
		fmt.Printf("%-40s ", name)
		if err := fn(); err != nil {
			fmt.Printf("FAIL  %v\n", err)
			fail++
		} else {
			fmt.Println("PASS")
			pass++
		}
	}

	run("Google connectivity (generate_204)", func() error {
		return check204(client, "https://www.google.com/generate_204")
	})

	run("YouTube connectivity (generate_204)", func() error {
		return check204(client, "https://www.youtube.com/generate_204")
	})

	run("InnerTube player (dQw4w9WgXcQ)", func() error {
		return checkInnerTubePlayer(client)
	})

	run("InnerTube search (python tutorial)", func() error {
		return checkInnerTubeSearch(client)
	})

	fmt.Printf("\n%d passed, %d failed\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

func check204(client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		return fmt.Errorf("want 204, got %d", resp.StatusCode)
	}
	return nil
}

// innerTubeClients lists client variants to try in order, mirroring test_fronting.py.
// Each entry: {name, apiKey, clientVersion, extra fields}.
var innerTubeClients = []struct {
	name    string
	apiKey  string
	version string
	extra   map[string]any
}{
	{"ANDROID", "AIzaSyA8eiZmM1FaDVjRy-df2KTyQ_vz_yYM394", "19.09.37", map[string]any{"androidSdkVersion": 30}},
	{"TVHTML5", "AIzaSyDCU8hByM-4DrUqRUYnGn-3llEO78bcxq8", "7.20230405", nil},
	{"IOS", "AIzaSyB-63vPrdThhKuerbB2N_l7Kwwcxj6yUAc", "19.09.3", nil},
	{"WEB", "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8", "2.20230101", nil},
	{"MWEB", "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8", "2.20230101", nil},
}

func checkInnerTubePlayer(client *http.Client) error {
	var lastErr error
	for _, c := range innerTubeClients {
		ctx := map[string]any{
			"clientName":    c.name,
			"clientVersion": c.version,
			"hl":            "en",
			"gl":            "US",
		}
		for k, v := range c.extra {
			ctx[k] = v
		}

		body, _ := json.Marshal(map[string]any{
			"videoId": "dQw4w9WgXcQ",
			"context": map[string]any{"client": ctx},
		})

		url := "https://www.youtube.com/youtubei/v1/player?key=" + c.apiKey
		req, _ := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data) //nolint:errcheck
		resp.Body.Close()

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("client %s: status %d", c.name, resp.StatusCode)
			continue
		}

		if _, ok := data["videoDetails"]; ok {
			if _, hasStream := data["streamingData"]; !hasStream {
				fmt.Printf("[%s: videoDetails only] ", c.name)
			} else {
				fmt.Printf("[%s] ", c.name)
			}
			return nil
		}
		lastErr = fmt.Errorf("client %s: no videoDetails in response", c.name)
	}
	return fmt.Errorf("all clients failed, last error: %v", lastErr)
}

func checkInnerTubeSearch(client *http.Client) error {
	body, _ := json.Marshal(map[string]any{
		"query": "python tutorial",
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20240101",
			},
		},
	})
	req, _ := http.NewRequestWithContext(context.Background(), "POST",
		"https://www.youtube.com/youtubei/v1/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	b, _ := io.ReadAll(resp.Body)
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	if _, ok := data["estimatedResults"]; !ok {
		if _, ok := data["contents"]; !ok {
			return fmt.Errorf("no estimatedResults or contents in response")
		}
	}
	return nil
}
