package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	Id       int    `json:"id"`
	Hostname string `json:"hostname"`
}

type Data struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// scanPort пытается установить соединение с указанным портом
func scanPort(protocol, hostname string, port int) bool {
	address := fmt.Sprintf("%s:%d", hostname, port)
	conn, err := net.DialTimeout(protocol, address, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func parseFileSystem(directory string) {
	files, err := os.ReadDir(directory)
	if err != nil {
		fmt.Println("Error of ReadDir: ", err)
	}
	for _, file := range files {
		if file.IsDir() {
			parseFileSystem(directory + file.Name() + "/")
		} else {
			fmt.Println("Directory:", directory, "File:", file)
		}
	}

}

func getInstalledAppsWindows() ([]string, error) {
	// Выполняем команду wmic product get name
	cmd := exec.Command("wmic", "product", "get", "name")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// Парсим вывод и убираем пустые строки
	lines := strings.Split(out.String(), "\n")
	var apps []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if (line != "") && (line != "Name") {
			apps = append(apps, line)
		}
	}
	return apps, nil
}

func WriteToFile(data []byte, filename string) bool {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0777)
	defer file.Close()
	if err != nil {
		fmt.Println("Error of opening file is:", err)
		return false
	}
	_, err = file.Write(data)
	if err != nil {
		return false
	}
	return true
}

func SendToServer(filename string, server string) {

	agentIdentificator := &Agent{
		Id:       1,
		Hostname: "test_agent",
	}
	file, err := os.OpenFile(filename, os.O_RDONLY, 0777)
	if err != nil {
		fmt.Println("Error of opening data file to read:", err)
		return
	}
	defer file.Close()

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	formFile, err := writer.CreateFormFile("data", filename)
	if err != nil {
		fmt.Println("Error of creating form: ", err)
		return
	}

	_, err = io.Copy(formFile, file)
	if err != nil {
		fmt.Println("Error of copying:", err)
		return
	}

	err = writer.Close()
	if err != nil {
		fmt.Println("Error of closing writer:", err)
		return
	}

	request, err := http.NewRequest("POST", server, &buffer)
	if err != nil {
		fmt.Println("Error of creating request to server:", err)
		return
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("AgentID", strconv.Itoa(agentIdentificator.Id))
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println("Error of sending request:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status, resp.Header, resp.Body)

}

func main() {
	//parseFileSystem("C://")
	hostname := "localhost"
	protocol := "tcp"
	startPort := 1
	endPort := 65536
	var wg sync.WaitGroup

	agentIdentificator := &Agent{
		Id:       1,
		Hostname: "test_agent",
	}

	go func() {
		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		syncBodyBytes, err := json.MarshalIndent(agentIdentificator, "", "")
		if err != nil {
			fmt.Println("Error of json marchal", err)
		}
		syncBody := bytes.NewReader(syncBodyBytes)
		for {
			scanTimer := time.NewTimer(time.Minute)
			<-scanTimer.C
			resp, err := client.Post("http://localhost:8090/synchronization", "application/json", syncBody)
			if err != nil {
				fmt.Println("Error of sending sync message", err)
				continue
			}
			if resp.StatusCode == 200 {
				fmt.Println("Success sync")
			} else {
				fmt.Println("Unsuccess sync")
			}
		}

	}()
	for {

		wg.Add(2)
		err := os.Remove("collected_info.dat.last")
		if err != nil {
			fmt.Println("Error of remove collected_info.dat.last", err)

		}

		err = os.Rename("collected_info.dat", "collected_info.dat.last")
		if err != nil {
			fmt.Println("Error of rename collected_info.dat", err)

		}

		_, err = os.Create("collected_info.dat")
		if err != nil {
			fmt.Println("Error of creating collected_info.dat", err)
			return
		}

		go func() {
			defer wg.Done()
			for port := startPort; port <= endPort; port++ {
				//wg.Add(1)
				if scanPort(protocol, hostname, port) {
					portJson := Data{
						Type:  "Port",
						Value: strconv.Itoa(port),
					}
					jsonData, err := json.MarshalIndent(portJson, "", "    ")
					if err != nil {
						fmt.Println("JSON Marchal error is", err)
					}
					WriteToFile(jsonData, "collected_info.dat")
					WriteToFile([]byte("\n"), "collected_info.dat")
				}

			}
		}()
		go func() {
			defer wg.Done()
			apps, err := getInstalledAppsWindows()

			if err != nil {
				fmt.Println("Error:", err)
			} else {
				for _, app := range apps {
					appJson := Data{
						Type:  "App",
						Value: app,
					}
					jsonData, err := json.MarshalIndent(appJson, "", "    ")
					if err != nil {
						fmt.Println("JSON Marchal error is", err)
					}
					WriteToFile(jsonData, "collected_info.dat")
					WriteToFile([]byte("\n"), "collected_info.dat")
				}
			}
		}()
		wg.Wait()
		fmt.Println("Scanning complete")

		SendToServer("collected_info.dat", "http://localhost:8090/get_res")

		scanTimer := time.NewTimer(time.Minute)
		<-scanTimer.C
	}

}
