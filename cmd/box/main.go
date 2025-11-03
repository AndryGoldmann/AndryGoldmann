package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/YouEclipse/steam-box/pkg/steambox"
)

// This program updates Markdown files with gaming statistics from Steam and Xbox Live (XBL) based on environment variables.
// It fetches data from the respective APIs and inserts or updates sections in specified Markdown files (e.g., for Steam playtime/recent games and XBL profile stats).
// The Steam section uses a custom library (steambox) to retrieve playtime leaderboards or recently played games and update a Markdown file between specific comment markers.
// The XBL section fetches user profile and recent activity via HTTP requests to the xbl.io API and updates the README.md file with formatted stats between markers.
// Environment variables control API keys, user IDs, options, and file paths. If keys are missing, the respective logic is skipped.

func main() {
	// Steam Logic: This section handles Steam API interactions if the STEAM_API_KEY environment variable is set.
	if steamAPIKey := os.Getenv("STEAM_API_KEY"); steamAPIKey != "" {
		var err error
		// Parse the Steam user ID from the STEAM_ID environment variable.
		steamID, _ := strconv.ParseUint(os.Getenv("STEAM_ID"), 10, 64)
		// Get the list of app IDs from APP_ID environment variable (comma-separated).
		appIDs := os.Getenv("APP_ID")
		appIDList := make([]uint32, 0)
		// Split the app IDs string and convert each to uint32, skipping any invalid ones.
		for _, appID := range strings.Split(appIDs, ",") {
			appid, err := strconv.ParseUint(appID, 10, 32)
			if err != nil {
				continue
			}
			appIDList = append(appIDList, uint32(appid))
		}
		// Set the Steam option for fetching games: ALLTIME (default), RECENT, or ALLTIME_AND_RECENT.
		// This determines whether to fetch all-time playtime, recent games, or both.
		steamOption := "ALLTIME" // options for types of games to list: RECENT (recently played games), ALLTIME <default> (playtime of games in descending order), ALLTIME_AND_RECENT for both
		if os.Getenv("STEAM_OPTION") != "" {
			steamOption = os.Getenv("STEAM_OPTION")
		}
		// Determine if the output should be multi-lined (hours on separate lines) based on MULTILINE env var.
		multiLined := false // boolean for whether hours should have their own line - YES = true, NO = false
		if os.Getenv("MULTILINE") != "" {
			lineOption := os.Getenv("MULTILINE")
			if lineOption == "YES" {
				multiLined = true
			}
		}
		// Get the Markdown file path from MARKDOWN_FILE env var (e.g., "MYFILE.md").
		markdownFile := os.Getenv("MARKDOWN_FILE") // the markdown filename (e.g. MYFILE.md)
		// Flags to determine if we need to update all-time playtime and/or recent games based on steamOption.
		updateAllTime := steamOption == "ALLTIME" || steamOption == "ALLTIME_AND_RECENT"
		updateRecent := steamOption == "RECENT" || steamOption == "ALLTIME_AND_RECENT"
		// Initialize the steambox client with the API key.
		box := steambox.NewBox(steamAPIKey)
		ctx := context.Background()
		var (
			filename string
			lines    []string
		)
		// If updating all-time playtime, fetch the data and update the Markdown if specified.
		if updateAllTime {
			// Set the section title for all-time playtime.
			filename = "🎮 Steam playtime leaderboard"
			// Fetch playtime data for the user and specified app IDs (if any).
			lines, err = box.GetPlayTime(ctx, steamID, multiLined, appIDList...)
			if err != nil {
				panic("GetPlayTime err:" + err.Error())
			}
			// If a Markdown file is specified, prepare the content and update the section between markers.
			if markdownFile != "" {
				content := bytes.NewBuffer(nil)
				content.WriteString(strings.Join(lines, "\n"))
				// Define start and end markers for the playtime section in the Markdown.
				start := []byte("<!-- steam-box-playtime start -->")
				end := []byte("<!-- steam-box-playtime end -->")
				// Update the Markdown file with the new content between markers.
				err = box.UpdateMarkdown(ctx, filename, markdownFile, content.Bytes(), start, end)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println("updating markdown successfully on ", markdownFile)
			}
		}
		// If updating recent games, fetch the data and update the Markdown if specified.
		if updateRecent {
			// Set the section title for recent games.
			filename = "🎮 Recently played Steam games"
			// Fetch recently played games for the user.
			lines, err = box.GetRecentGames(ctx, steamID, multiLined)
			if err != nil {
				panic("GetRecentGames err:" + err.Error())
			}
			// If a Markdown file is specified, prepare the content and update the section between markers.
			if markdownFile != "" {
				content := bytes.NewBuffer(nil)
				content.WriteString(strings.Join(lines, "\n"))
				// Define start and end markers for the recent games section in the Markdown.
				start := []byte("<!-- steam-box-recent start -->")
				end := []byte("<!-- steam-box-recent end -->")
				// Update the Markdown file with the new content between markers.
				err = box.UpdateMarkdown(ctx, filename, markdownFile, content.Bytes(), start, end)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println("updating markdown successfully on ", markdownFile)
			}
		}
	}

	// XBL Logic: This section handles Xbox Live API interactions if the XBL_API_KEY environment variable is set.
	if xblKey := os.Getenv("XBL_API_KEY"); xblKey != "" {
		// Get the Xbox User ID (XUID) from the XBL_XUID env var.
		xuid := os.Getenv("XBL_XUID")
		if xuid == "" {
			fmt.Println("XBL: Missing XBL_XUID env var")
			return
		}
		// Create an HTTP client for API requests.
		client := &http.Client{}
		// Fetch the user's profile from the xbl.io API.
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://xbl.io/api/v2/account/%s", xuid), nil)
		req.Header.Set("X-Authorization", xblKey)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("XBL: Error fetching profile: %v\n", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("XBL: Profile API error %d: %s\n", resp.StatusCode, string(body))
			return
		}
		// Read and debug-print the profile response body.
		profileBody, _ := io.ReadAll(resp.Body)
		fmt.Println("DEBUG - Profile JSON:", string(profileBody)) // Debug print
		// Define a struct to unmarshal the profile JSON.
		var pr struct {
			ProfileUsers []struct {
				Id         string `json:"id"`
				Gamertag   string `json:"gamertag"`
				Gamerscore string `json:"gamerscore"`
				Gamerpic   string `json:"gamerPic"`
			} `json:"profileUsers"`
		}
		// Unmarshal the profile data.
		if err := json.Unmarshal(profileBody, &pr); err != nil {
			fmt.Printf("XBL: Error parsing profile: %v\n", err)
			return
		}
		if len(pr.ProfileUsers) == 0 {
			fmt.Println("XBL: No profile users found")
			return
		}
		// Extract the first profile user's data.
		profile := pr.ProfileUsers[0]
		// Parse gamerscore to int, default to 0 on error.
		gamerscore, err := strconv.Atoi(profile.Gamerscore)
		if err != nil {
			gamerscore = 0
			fmt.Printf("XBL: Error parsing gamerscore: %v\n", err)
		}
		// Fetch the user's recent activity feed from the xbl.io API.
		req2, _ := http.NewRequest("GET", "https://xbl.io/api/v2/activity/feed", nil)
		req2.Header.Set("X-Authorization", xblKey)
		resp2, err := client.Do(req2)
		// Define a struct to unmarshal the recent activity JSON (focusing on title associations).
		var recent []struct {
			TitleAssociations []struct {
				Name string `json:"name"`
			} `json:"titleAssociations"`
		}
		if err == nil && resp2.StatusCode == 200 {
			defer resp2.Body.Close()
			recentBody, _ := io.ReadAll(resp2.Body)
			fmt.Println("DEBUG - Recent Activity JSON:", string(recentBody)) // Debug print
			// Unmarshal the recent activity data.
			if err := json.Unmarshal(recentBody, &recent); err != nil {
				fmt.Printf("XBL: Error parsing recent activity: %v\n", err)
			}
		} else if err != nil {
			fmt.Printf("XBL: Error fetching recent activity: %v\n", err)
		} else {
			fmt.Printf("XBL: Recent activity API error %d\n", resp2.StatusCode)
		}
		// Collect up to 5 unique recent games from the activity feed.
		games := make(map[string]bool)
		var gameList []string
		count := 0
		for _, act := range recent {
			if count >= 5 {
				break
			}
			for _, title := range act.TitleAssociations {
				if !games[title.Name] {
					games[title.Name] = true
					gameList = append(gameList, fmt.Sprintf("- %s", title.Name))
					count++
					if count >= 5 {
						break
					}
				}
			}
		}
		// Join the game list into a string.
		gamesStr := strings.Join(gameList, "\n")
		// Format the XBL stats section as Markdown.
		stats := fmt.Sprintf("### Xbox Live Stats\n![Avatar](%s)\n**Gamerscore:** %d\n**Gamertag:** %s\n\n**Recent Games:**\n%s", profile.Gamerpic, gamerscore, profile.Gamertag, gamesStr)
		// Read the contents of README.md.
		readme, err := os.ReadFile("README.md")
		if err != nil {
			fmt.Printf("XBL: Error reading README: %v\n", err)
			return
		}
		str := string(readme)
		// Use regex to find and replace the content between <!-- XBL_STATS --> and <!-- /XBL_STATS --> markers.
		re := regexp.MustCompile(`(?s)<!-- XBL_STATS -->.*?<!-- /XBL_STATS -->`)
		newStr := re.ReplaceAllString(str, `<!-- XBL_STATS -->`+strings.TrimSpace(stats)+`<!-- /XBL_STATS -->`)
		// Write the updated content back to README.md.
		if err := os.WriteFile("README.md", []byte(newStr), 0644); err != nil {
			fmt.Printf("XBL: Error writing README: %v\n", err)
			return
		}
		fmt.Println("XBL: README updated with XBL stats!")
	}
}
