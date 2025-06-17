package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	// "path/filepath"
	// "strconv"
	"strings"
	// "time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// Data structures
type Application struct {
	AppName   []string `json:"app_name"`
	Publisher []string `json:"publisher"`
}

type Category struct {
	Category     string        `json:"category"`
	Applications []Application `json:"applications"`
}

type LinuxData struct {
	LinuxCategories []Category `json:"linuxCategories"`
}

type Stats struct {
	TotalCategories   int            `json:"totalCategories"`
	TotalApplications int            `json:"totalApplications"`
	CategoryCounts    map[string]int `json:"categoryCounts"`
	RecentlyAdded     int            `json:"recentlyAdded"`
}

type AddApplicationRequest struct {
	Category  string   `json:"category"`
	AppName   []string `json:"app_name"`
	Publisher []string `json:"publisher"`
}

type AddCategoryRequest struct {
	Category string `json:"category"`
}

var (
	dataFile = "data.json"
	data     LinuxData
)

func main() {
	// Initialize data
	loadData()

	// Create router
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/stats", getStats).Methods("GET")
	api.HandleFunc("/categories", getCategories).Methods("GET")
	api.HandleFunc("/applications", getApplications).Methods("GET")
	api.HandleFunc("/applications", addApplication).Methods("POST")
	api.HandleFunc("/categories", addCategory).Methods("POST")
	api.HandleFunc("/download", downloadData).Methods("GET")
	api.HandleFunc("/download/{category}", downloadByCategory).Methods("GET")
	api.HandleFunc("/upload", uploadData).Methods("POST")
	api.HandleFunc("/delete/application", deleteApplication).Methods("DELETE")
	api.HandleFunc("/delete/category", deleteCategory).Methods("DELETE")

	// Serve static files
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/"))).Methods("GET")

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	handler := c.Handler(r)

	fmt.Println("🚀 Server starting on http://localhost:8080")
	fmt.Println("📊 Dashboard available at http://localhost:8080")
	// log.Fatal(http.ListenAndServe(":8080", handler))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, handler))
}

func loadData() {
	file, err := os.Open(dataFile)
	if err != nil {
		// Create default data if file doesn't exist
		data = LinuxData{
			LinuxCategories: []Category{
				{
					Category: "VPN",
					Applications: []Application{
						{
							AppName:   []string{"gimp"},
							Publisher: []string{"Snapcrafters"},
						},
						{
							AppName:   []string{"wireguard"},
							Publisher: []string{"Snapcrafters"},
						},
					},
				},
				{
					Category: "Security",
					Applications: []Application{
						{
							AppName:   []string{"wireguard"},
							Publisher: []string{"Snapcrafters"},
						},
					},
				},
			},
		}
		saveData()
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		return
	}
}

func saveData() {
	file, err := os.Create(dataFile)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(data); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

func getStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	totalCategories := len(data.LinuxCategories)
	totalApplications := 0
	categoryCounts := make(map[string]int)

	for _, category := range data.LinuxCategories {
		appCount := len(category.Applications)
		totalApplications += appCount
		categoryCounts[category.Category] = appCount
	}

	stats := Stats{
		TotalCategories:   totalCategories,
		TotalApplications: totalApplications,
		CategoryCounts:    categoryCounts,
		RecentlyAdded:     0, // Since we don't have timestamps, set to 0
	}

	json.NewEncoder(w).Encode(stats)
}

func getCategories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	categories := make([]string, 0, len(data.LinuxCategories))
	for _, category := range data.LinuxCategories {
		categories = append(categories, category.Category)
	}

	json.NewEncoder(w).Encode(categories)
}

func getApplications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	category := r.URL.Query().Get("category")
	
	if category == "" {
		// Return all applications
		json.NewEncoder(w).Encode(data)
		return
	}

	// Return applications for specific category
	for _, cat := range data.LinuxCategories {
		if cat.Category == category {
			json.NewEncoder(w).Encode(cat)
			return
		}
	}

	// Category not found
	http.Error(w, "Category not found", http.StatusNotFound)
}

func addApplication(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req AddApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Category == "" || len(req.AppName) == 0 || len(req.Publisher) == 0 {
		http.Error(w, "Category, app_name, and publisher are required", http.StatusBadRequest)
		return
	}

	// Find or create category
	var targetCategory *Category
	for i := range data.LinuxCategories {
		if data.LinuxCategories[i].Category == req.Category {
			targetCategory = &data.LinuxCategories[i]
			break
		}
	}

	if targetCategory == nil {
		// Create new category
		newCategory := Category{
			Category:     req.Category,
			Applications: []Application{},
		}
		data.LinuxCategories = append(data.LinuxCategories, newCategory)
		targetCategory = &data.LinuxCategories[len(data.LinuxCategories)-1]
	}

	// Check if application already exists
	for _, app := range targetCategory.Applications {
		if equalStringSlices(app.AppName, req.AppName) && equalStringSlices(app.Publisher, req.Publisher) {
			http.Error(w, "Application already exists in this category", http.StatusConflict)
			return
		}
	}

	// Add application
	newApp := Application{
		AppName:   req.AppName,
		Publisher: req.Publisher,
	}
	targetCategory.Applications = append(targetCategory.Applications, newApp)

	saveData()

	response := map[string]string{"message": "Application added successfully"}
	json.NewEncoder(w).Encode(response)
}

func addCategory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req AddCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Category == "" {
		http.Error(w, "Category name is required", http.StatusBadRequest)
		return
	}

	// Check if category already exists
	for _, category := range data.LinuxCategories {
		if category.Category == req.Category {
			http.Error(w, "Category already exists", http.StatusConflict)
			return
		}
	}

	// Add new category
	newCategory := Category{
		Category:     req.Category,
		Applications: []Application{},
	}
	data.LinuxCategories = append(data.LinuxCategories, newCategory)

	saveData()

	response := map[string]string{"message": "Category added successfully"}
	json.NewEncoder(w).Encode(response)
}

func downloadData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=linux_signatures.json")

	json.NewEncoder(w).Encode(data)
}

func downloadByCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	categoryName := vars["category"]

	for _, category := range data.LinuxCategories {
		if category.Category == categoryName {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_signatures.json", strings.ToLower(strings.ReplaceAll(categoryName, " ", "_"))))
			
			categoryData := LinuxData{
				LinuxCategories: []Category{category},
			}
			json.NewEncoder(w).Encode(categoryData)
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}

func uploadData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10MB max
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("jsonFile")
	if err != nil {
		http.Error(w, "Unable to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Unable to read file", http.StatusInternalServerError)
		return
	}

	// Parse JSON
	var uploadedData LinuxData
	if err := json.Unmarshal(fileBytes, &uploadedData); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Merge data
	addedApps := 0
	addedCategories := 0

	for _, uploadedCategory := range uploadedData.LinuxCategories {
		// Find existing category or create new one
		var existingCategory *Category
		for i := range data.LinuxCategories {
			if data.LinuxCategories[i].Category == uploadedCategory.Category {
				existingCategory = &data.LinuxCategories[i]
				break
			}
		}

		if existingCategory == nil {
			// Create new category
			data.LinuxCategories = append(data.LinuxCategories, Category{
				Category:     uploadedCategory.Category,
				Applications: []Application{},
			})
			existingCategory = &data.LinuxCategories[len(data.LinuxCategories)-1]
			addedCategories++
		}

		// Add applications
		for _, uploadedApp := range uploadedCategory.Applications {
			// Check if application already exists
			exists := false
			for _, existingApp := range existingCategory.Applications {
				if equalStringSlices(existingApp.AppName, uploadedApp.AppName) &&
					equalStringSlices(existingApp.Publisher, uploadedApp.Publisher) {
					exists = true
					break
				}
			}

			if !exists {
				existingCategory.Applications = append(existingCategory.Applications, uploadedApp)
				addedApps++
			}
		}
	}

	saveData()

	response := map[string]interface{}{
		"message":          "Data uploaded successfully",
		"addedCategories":  addedCategories,
		"addedApplications": addedApps,
	}
	json.NewEncoder(w).Encode(response)
}

func deleteApplication(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	category := r.URL.Query().Get("category")
	appName := r.URL.Query().Get("app_name")
	
	if category == "" || appName == "" {
		http.Error(w, "Category and app_name parameters are required", http.StatusBadRequest)
		return
	}

	// Find category and application
	for i, cat := range data.LinuxCategories {
		if cat.Category == category {
			for j, app := range cat.Applications {
				if len(app.AppName) > 0 && app.AppName[0] == appName {
					// Remove application
					data.LinuxCategories[i].Applications = append(
						data.LinuxCategories[i].Applications[:j],
						data.LinuxCategories[i].Applications[j+1:]...,
					)
					saveData()
					
					response := map[string]string{"message": "Application deleted successfully"}
					json.NewEncoder(w).Encode(response)
					return
				}
			}
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}

func deleteCategory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	category := r.URL.Query().Get("category")
	
	if category == "" {
		http.Error(w, "Category parameter is required", http.StatusBadRequest)
		return
	}

	// Find and remove category
	for i, cat := range data.LinuxCategories {
		if cat.Category == category {
			data.LinuxCategories = append(data.LinuxCategories[:i], data.LinuxCategories[i+1:]...)
			saveData()
			
			response := map[string]string{"message": "Category deleted successfully"}
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	http.Error(w, "Category not found", http.StatusNotFound)
}

// Helper function to compare string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}