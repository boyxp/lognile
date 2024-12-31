# Lognile
Lightweight File Log Collection Tool for Golang

![](https://img.shields.io/npm/l/vue.svg)


Lognile is a lightweight log collection tool implemented in Go. It allows you to monitor log files and automatically handle new log entries.

## Quick Start

1. **Create a Configuration File:**
   Create a file named `config.yaml` with the following content:

   ```yaml
   # Paths to the log files to monitor
   pattern:
       - ./_log/php-fpm-*.log
       - ./_log/nginx-*.log

   # File to store log progress
   db: lognile.db
   ```

2. **Create a Basic Application:**
   Create a file named `main.go` with the following content to use Lognile:

   ```go
   package main

   import (
       "log"
       "github.com/boyxp/lognile"
   )

   func main() {
       // Initialize Lognile with the configuration file
       lognile.Init("config.yaml", func(row map[string]string) {
           // Handle new log entries
           log.Println(row)
       })
   }
   ```

3. **Run the Application:**
   Use the following command to run your application:

   ```sh
   go run main.go
   ```

## Explanation

- **Configuration File (`config.yaml`):**
  - `pattern`: Specifies the paths to the log files to monitor. Wildcards can be used to match multiple files.
  - `db`: Specifies the file used to store log progress, so it can resume from the last position after restarting.

- **Application (`main.go`):**
  - **Import Packages**: Import the `log` package for logging and the `lognile` package for log collection.
  - **Initialize Lognile**: Call `lognile.Init` with the path to `config.yaml` and a callback function to handle new log entries.
  - **Callback Function**: The callback function receives a map containing the new log entry. In this example, it simply logs the entry using `log.Println`.

### Running the Application
- **Start Monitoring**: Once the application runs, it will start monitoring the specified log files for new entries.
- **Handle Log Entries**: New log entries will be passed to the callback function, where you can process them as needed.

## Demo
Please see the [demo](_demo) for a demonstration of its actual performance.
