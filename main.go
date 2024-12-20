package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const GITHUB_REPO = "git@github.com:dharshan-kumarj/Cratoss.git"

type PageData struct {
	Title     string
	Error     string
	Username  string
	Cloned    bool
	Workspace string
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
        }
        .form-container {
            background-color: #f5f5f5;
            padding: 20px;
            border-radius: 5px;
            margin-top: 20px;
        }
        input[type="text"] {
            width: 100%;
            padding: 10px;
            margin: 10px 0;
            border: 1px solid #ddd;
            border-radius: 4px;
        }
        button {
            background-color: #4CAF50;
            color: white;
            padding: 10px 20px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            margin: 10px 0;
        }
        button:hover {
            background-color: #45a049;
        }
        .error {
            color: red;
            margin-top: 10px;
        }
        #status-header {
            background-color: #333;
            color: white;
            padding: 10px;
            margin-bottom: 20px;
            border-radius: 4px;
            display: none;
        }
        .hidden {
            display: none;
        }
    </style>
    <script>
        function startCloning() {
            const username = document.getElementById('username').value;
            if (!username) {
                alert('Please enter a username');
                return;
            }

            const statusHeader = document.getElementById('status-header');
            statusHeader.style.display = 'block';
            statusHeader.textContent = 'Checking workspace...';

            // First check if workspace exists
            fetch('/check-workspace?username=' + username)
                .then(response => response.json())
                .then(data => {
                    if (data.exists) {
                        // If workspace exists, redirect to it
                        window.location.href = '/workspace?username=' + username;
                    } else {
                        // If workspace doesn't exist, start cloning
                        const eventSource = new EventSource('/stream?username=' + username);
                        
                        eventSource.onmessage = function(event) {
                            statusHeader.textContent = event.data;
                            if (event.data.includes('Clone completed')) {
                                eventSource.close();
                                window.location.href = '/workspace?username=' + username;
                            }
                        };

                        document.getElementById('cloneForm').submit();
                    }
                });
        }
    </script>
</head>
<body>
    <h1>{{.Title}}</h1>
    <div id="status-header"></div>
    <div class="form-container">
        {{if not .Username}}
            <form id="cloneForm" action="/clone" method="POST">
                <div>
                    <label for="username">Enter Username:</label>
                    <input type="text" id="username" name="username" placeholder="your-username" required>
                </div>
                <button type="button" onclick="startCloning()">Clone Repository</button>
            </form>
        {{else}}
            <h2>Repository cloned for: {{.Username}}</h2>
            <p>Workspace: {{.Workspace}}</p>
            <form action="/open" method="POST">
                <input type="hidden" name="username" value="{{.Username}}">
                <button type="submit">Open in VSCode</button>
            </form>
        {{end}}
        {{if .Error}}
        <p class="error">{{.Error}}</p>
        {{end}}
    </div>
</body>
</html>
`

func main() {
	tmpl := template.Must(template.New("index").Parse(htmlTemplate))

	// New handler to check if workspace exists
	http.HandleFunc("/check-workspace", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		if username == "" {
			http.Error(w, "Username is required", http.StatusBadRequest)
			return
		}

		workspacePath := username
		exists := false
		if _, err := os.Stat(workspacePath); err == nil {
			// Check if it's a git repository
			if _, err := os.Stat(workspacePath + "/.git"); err == nil {
				exists = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"exists": %v}`, exists)
	})

	// Handle streaming updates
	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		if username == "" {
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		updates := []string{
			"Preparing workspace...",
			"Initializing Git...",
			"Cloning repository...",
			"Setting up workspace...",
			"Clone completed successfully!",
		}

		for _, update := range updates {
			fmt.Fprintf(w, "data: %s\n\n", update)
			w.(http.Flusher).Flush()
			time.Sleep(1 * time.Second)
		}
	})

	// Handle main page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		data := PageData{
			Title:    "GitHub Repository Cloner",
			Username: username,
		}
		if username != "" {
			data.Workspace = username
		}
		tmpl.Execute(w, data)
	})

	// Handle clone request
	http.HandleFunc("/clone", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		username := r.FormValue("username")
		if username == "" {
			handleError(w, tmpl, "Username is required", nil)
			return
		}

		workspacePath := username

		// Check if directory already exists and is a git repository
		if _, err := os.Stat(workspacePath); err == nil {
			if _, err := os.Stat(workspacePath + "/.git"); err == nil {
				// Directory exists and is a git repository, redirect to workspace
				http.Redirect(w, r, fmt.Sprintf("/workspace?username=%s", username), http.StatusSeeOther)
				return
			}
		}

		// Create directory with username
		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			handleError(w, tmpl, "Failed to create workspace", err)
			return
		}

		// Clone repository
		cmd := exec.Command("git", "clone", GITHUB_REPO, workspacePath)
		if err := cmd.Run(); err != nil {
			handleError(w, tmpl, "Failed to clone repository", err)
			return
		}

		// Redirect to workspace page
		http.Redirect(w, r, fmt.Sprintf("/workspace?username=%s", username), http.StatusSeeOther)
	})

	// Handle workspace page
	http.HandleFunc("/workspace", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		data := PageData{
			Title:     "GitHub Repository Cloner",
			Username:  username,
			Workspace: username,
		}
		tmpl.Execute(w, data)
	})

	// Handle VSCode open request
	http.HandleFunc("/open", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		username := r.FormValue("username")
		workspacePath := username

		cmd := exec.Command("code", workspacePath)
		if err := cmd.Run(); err != nil {
			handleError(w, tmpl, "Failed to open VSCode", err)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/?username=%s", username), http.StatusSeeOther)
	})

	fmt.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleError(w http.ResponseWriter, tmpl *template.Template, message string, err error) {
	data := PageData{
		Title: "GitHub Repository Cloner",
		Error: fmt.Sprintf("%s: %v", message, err),
	}
	tmpl.Execute(w, data)
}
