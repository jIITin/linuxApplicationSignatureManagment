package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Application represents an application with its name and publisher.
type Application struct {
	AppName   []string `json:"app_name"`
	Publisher []string `json:"publisher"`
}

// Category represents a category with its applications.
type Category struct {
	Category     string        `json:"category"`
	Applications []Application `json:"applications"`
}

// DataStore holds the application data and recent additions.
type DataStore struct {
	LinuxCategories []Category           `json:"linuxCategories"`
	RecentApps      map[string]time.Time // Tracks recently added apps
	mu              sync.RWMutex         // For thread-safe operations
}

// Global in-memory data store
var store = DataStore{
	LinuxCategories: []Category{},
	RecentApps:      make(map[string]time.Time),
}

func main() {
	// Serve static files (index.html, Chart.js, etc.)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Serve index.html at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})

	// API endpoints
	http.HandleFunc("/api/stats", statsHandler)
	http.HandleFunc("/api/categories", categoriesHandler)
	http.HandleFunc("/api/applications", applicationsHandler)
	http.HandleFunc("/api/search", searchHandler)
	http.HandleFunc("/api/upload", uploadHandler)
	http.HandleFunc("/api/download/", downloadHandler)
	http.HandleFunc("/api/update/application", updateApplicationHandler)
	http.HandleFunc("/api/delete/application", deleteApplicationHandler)

	// Start server
	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// statsHandler returns statistics about categories and applications.
func statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	categoryCounts := make(map[string]int)
	for _, category := range store.LinuxCategories {
		categoryCounts[category.Category] = len(category.Applications)
	}

	recentlyAdded := 0
	for _, t := range store.RecentApps {
		if time.Since(t) < 24*time.Hour {
			recentlyAdded++
		}
	}

	totalApps := 0
	for _, category := range store.LinuxCategories {
		totalApps += len(category.Applications)
	}

	stats := struct {
		TotalCategories   int            `json:"totalCategories"`
		TotalApplications int            `json:"totalApplications"`
		RecentlyAdded     int            `json:"recentlyAdded"`
		CategoryCounts    map[string]int `json:"categoryCounts"`
	}{
		TotalCategories:   len(store.LinuxCategories),
		TotalApplications: totalApps,
		RecentlyAdded:     recentlyAdded,
		CategoryCounts:    categoryCounts,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// categoriesHandler handles GET (list categories) and POST (add category).
func categoriesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store.mu.RLock()
		defer store.mu.RUnlock()

		categories := make([]string, 0, len(store.LinuxCategories))
		for _, category := range store.LinuxCategories {
			categories = append(categories, category.Category)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(categories); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

	case http.MethodPost:
		var categoryData struct {
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&categoryData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if categoryData.Category == "" {
			http.Error(w, "Category name is required", http.StatusBadRequest)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		for _, cat := range store.LinuxCategories {
			if strings.EqualFold(cat.Category, categoryData.Category) {
				http.Error(w, "Category already exists", http.StatusConflict)
				return
			}
		}

		store.LinuxCategories = append(store.LinuxCategories, Category{
			Category:     categoryData.Category,
			Applications: []Application{},
		})

		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// applicationsHandler handles GET (list applications) and POST (add application).
func applicationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store.mu.RLock()
		defer store.mu.RUnlock()

		category := r.URL.Query().Get("category")
		if category == "" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(struct {
				LinuxCategories []Category `json:"linuxCategories"`
			}{store.LinuxCategories}); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
			return
		}

		for _, cat := range store.LinuxCategories {
			if strings.EqualFold(cat.Category, category) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(cat); err != nil {
					http.Error(w, "Failed to encode response", http.StatusInternalServerError)
				}
				return
			}
		}

		http.Error(w, "Category not found", http.StatusNotFound)

	case http.MethodPost:
		var appData struct {
			Category  string   `json:"category"`
			AppName   []string `json:"app_name"`
			Publisher []string `json:"publisher"`
		}
		if err := json.NewDecoder(r.Body).Decode(&appData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if appData.Category == "" || len(appData.AppName) == 0 || len(appData.Publisher) == 0 {
			http.Error(w, "Category, app_name, and publisher are required", http.StatusBadRequest)
			return
		}

		if len(appData.AppName) != len(appData.Publisher) {
			http.Error(w, "App names and publishers must have the same length", http.StatusBadRequest)
			return
		}

		store.mu.Lock()
		defer store.mu.Unlock()

		for i, cat := range store.LinuxCategories {
			if strings.EqualFold(cat.Category, appData.Category) {
				for _, app := range cat.Applications {
					for j, name := range app.AppName {
						if name == appData.AppName[0] && app.Publisher[j] == appData.Publisher[0] {
							http.Error(w, "Application already exists in this category", http.StatusConflict)
							return
						}
					}
				}
				store.LinuxCategories[i].Applications = append(store.LinuxCategories[i].Applications, Application{
					AppName:   appData.AppName,
					Publisher: appData.Publisher,
				})
				for i, name := range appData.AppName {
					store.RecentApps[fmt.Sprintf("%s:%s:%s", appData.Category, name, appData.Publisher[i])] = time.Now()
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		http.Error(w, "Category not found", http.StatusNotFound)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// searchHandler searches applications by name or publisher.
func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter is required", http.StatusBadRequest)
		return
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	matchingCategory := Category{
		Category:     "Search Results",
		Applications: []Application{},
	}

	query = strings.ToLower(query)
	for _, category := range store.LinuxCategories {
		for _, app := range category.Applications {
			for i, name := range app.AppName {
				if strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(app.Publisher[i]), query) {
					matchingCategory.Applications = append(matchingCategory.Applications, app)
					break
				}
			}
		}
	}

	if len(matchingCategory.Applications) == 0 {
		http.Error(w, "No applications found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(matchingCategory); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// uploadHandler handles JSON file uploads for bulk import.
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB limit
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("jsonFile")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	var uploadData struct {
		LinuxCategories []Category `json:"linuxCategories"`
	}
	if err := json.Unmarshal(data, &uploadData); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	addedCategories := 0
	addedApplications := 0

	for _, newCat := range uploadData.LinuxCategories {
		if newCat.Category == "" {
			continue
		}

		var existingCat *Category
		for i, cat := range store.LinuxCategories {
			if strings.EqualFold(cat.Category, newCat.Category) {
				existingCat = &store.LinuxCategories[i]
				break
			}
		}

		if existingCat == nil {
			store.LinuxCategories = append(store.LinuxCategories, Category{
				Category:     newCat.Category,
				Applications: []Application{},
			})
			existingCat = &store.LinuxCategories[len(store.LinuxCategories)-1]
			addedCategories++
		}

		for _, app := range newCat.Applications {
			if len(app.AppName) == 0 || len(app.Publisher) == 0 || len(app.AppName) != len(app.Publisher) {
				continue
			}

			exists := false
			for _, existingApp := range existingCat.Applications {
				for j, name := range existingApp.AppName {
					if name == app.AppName[0] && existingApp.Publisher[j] == app.Publisher[0] {
						exists = true
						break
					}
				}
				if exists {
					break
				}
			}

			if !exists {
				existingCat.Applications = append(existingCat.Applications, app)
				addedApplications += len(app.AppName)
				for i, name := range app.AppName {
					store.RecentApps[fmt.Sprintf("%s:%s:%s", newCat.Category, name, app.Publisher[i])] = time.Now()
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		AddedCategories   int `json:"addedCategories"`
		AddedApplications int `json:"addedApplications"`
	}{addedCategories, addedApplications}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// downloadHandler handles JSON downloads for all data or by category.
func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/download/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		// Download all data
		w.Header().Set("Content-Disposition", "attachment; filename=linux_signatures.json")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			LinuxCategories []Category `json:"linuxCategories"`
		}{store.LinuxCategories}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Download by category
	category := parts[0]
	for _, cat := range store.LinuxCategories {
		if strings.EqualFold(cat.Category, category) {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_signatures.json", strings.ToLower(strings.ReplaceAll(category, " ", "_"))))
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(cat); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}

// updateApplicationHandler updates an existing application.
func updateApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var updateData struct {
		Category     string   `json:"category"`
		OldAppName   string   `json:"old_app_name"`
		OldPublisher string   `json:"old_publisher"`
		NewAppName   []string `json:"new_app_name"`
		NewPublisher []string `json:"new_publisher"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if updateData.Category == "" || updateData.OldAppName == "" || updateData.OldPublisher == "" ||
		len(updateData.NewAppName) == 0 || len(updateData.NewPublisher) == 0 ||
		len(updateData.NewAppName) != len(updateData.NewPublisher) {
		http.Error(w, "All fields are required and new app names and publishers must match in length", http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	for i, cat := range store.LinuxCategories {
		if strings.EqualFold(cat.Category, updateData.Category) {
			for j, app := range cat.Applications {
				for k, name := range app.AppName {
					if name == updateData.OldAppName && app.Publisher[k] == updateData.OldPublisher {
						store.LinuxCategories[i].Applications[j].AppName[k] = updateData.NewAppName[0]
						store.LinuxCategories[i].Applications[j].Publisher[k] = updateData.NewPublisher[0]
						delete(store.RecentApps, fmt.Sprintf("%s:%s:%s", updateData.Category, updateData.OldAppName, updateData.OldPublisher))
						store.RecentApps[fmt.Sprintf("%s:%s:%s", updateData.Category, updateData.NewAppName[0], updateData.NewPublisher[0])] = time.Now()
						w.WriteHeader(http.StatusOK)
						return
					}
				}
			}
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}

// deleteApplicationHandler deletes an application from a category.
func deleteApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	category := r.URL.Query().Get("category")
	appName := r.URL.Query().Get("app_name")
	publisher := r.URL.Query().Get("publisher")

	if category == "" || appName == "" || publisher == "" {
		http.Error(w, "Category, app_name, and publisher are required", http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	for i, cat := range store.LinuxCategories {
		if strings.EqualFold(cat.Category, category) {
			for j, app := range cat.Applications {
				for k, name := range app.AppName {
					if name == appName && app.Publisher[k] == publisher {
						// Remove the specific app name and publisher
						store.LinuxCategories[i].Applications[j].AppName = append(app.AppName[:k], app.AppName[k+1:]...)
						store.LinuxCategories[i].Applications[j].Publisher = append(app.Publisher[:k], app.Publisher[k+1:]...)
						delete(store.RecentApps, fmt.Sprintf("%s:%s:%s", category, appName, publisher))
						// If no names left, remove the application
						if len(store.LinuxCategories[i].Applications[j].AppName) == 0 {
							store.LinuxCategories[i].Applications = append(store.LinuxCategories[i].Applications[:j], store.LinuxCategories[i].Applications[j+1:]...)
						}
						w.WriteHeader(http.StatusOK)
						return
					}
				}
			}
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}
