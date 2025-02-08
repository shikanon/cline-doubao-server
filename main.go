package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
)

// 长文本模型
var fixedTextLongModel = ""

// 文本模型
var fixedTextModel = ""

// 图像模型
var fixedVersionModel = ""

// 设置API密钥
var apiKey = ""

// 定义请求结构体
type TokenRequest struct {
	Model string   `json:"model"`
	Text  []string `json:"text"`
}

// 定义响应结构体
type TokenResponse struct {
	Object string `json:"object"`
	Data   []struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"data"`
}

// 获取token数量的方法
// apiURL: 接口地址
// apiKey: 认证令牌
// model: 模型名称
// texts: 输入的文本数组
func GetTotalTokens(model string, texts []string) (int, error) {
	apiURL := "https://ark.cn-beijing.volces.com/api/v3/tokenization"
	// 创建请求体
	reqBody := TokenRequest{
		Model: model,
		Text:  texts,
	}

	// 序列化请求体
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiKey)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("请求发送失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return 0, fmt.Errorf("响应解析失败: %v", err)
	}

	// 检查数据有效性
	if len(tokenResp.Data) == 0 {
		return 0, fmt.Errorf("响应数据为空")
	}

	return tokenResp.Data[0].TotalTokens, nil
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type OpenAIRequest struct {
	Model            string                 `json:"model"`
	Messages         []Message              `json:"messages"`
	MaxTokens        int                    `json:"max_tokens,omitempty"`
	Temperature      *float64               `json:"temperature,omitempty"`
	TopP             *float64               `json:"top_p,omitempty"`
	N                int                    `json:"n,omitempty"`
	Stream           bool                   `json:"stream,omitempty"`
	Stop             []string               `json:"stop,omitempty"`
	PresencePenalty  float64                `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64                `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]interface{} `json:"logit_bias,omitempty"`
	User             string                 `json:"user,omitempty"`
}

type ModelResponse struct {
	Models []string `json:"models"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	// 记录请求路径
	log.Printf("Request path: %s", r.URL.Path)

	// 设置CORS头信息，允许跨域请求
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// 提前处理OPTIONS请求，直接返回200 OK
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	token := r.Header.Get("Authorization")
	// log.Printf("Token: %s", token)
	apiKey = token
	if apiKey == "" {
		// 如果API密钥未设置，返回500 Internal Server Error
		http.Error(w, "API key not set", http.StatusInternalServerError)
		log.Println("Error: API key not set")
		return
	}

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		// 如果读取请求体失败，返回400 Bad Request
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		log.Printf("Error reading request body: %v", err)
		return
	}
	

	// 如果请求体为空，返回400 Bad Request
	if len(reqBody) == 0 {
		log.Println("Error: Empty request body")
		http.Error(w, "Empty request body", http.StatusBadRequest)
		return
	}

	var openaiReq OpenAIRequest
	if err := json.Unmarshal(reqBody, &openaiReq); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		log.Printf("Error parsing request body: %v", err)
		return
	}

	log.Printf("Model %s", openaiReq.Model)
	fixedTextLongModel=openaiReq.Model
	fixedTextModel=openaiReq.Model
	fixedVersionModel=openaiReq.Model
	// log.Printf("Parsed request: %+v", openaiReq)

	setDefaultParams(&openaiReq)

	messageArray := processMessages(openaiReq.Messages, &openaiReq)

	num, err := GetTotalTokens(fixedTextLongModel, messageArray)
	if err != nil && num > 25000 && openaiReq.Model != fixedVersionModel {
		openaiReq.Model = fixedTextLongModel
	}

	log.Printf("use model: %s, input text: %d", openaiReq.Model, num)

	newReqBody, err := json.Marshal(openaiReq)
	if err != nil {
		http.Error(w, "Failed to create new request body", http.StatusInternalServerError)
		log.Printf("Error creating new request body: %v", err)
		return
	}

	openaiURL := "https://ark.cn-beijing.volces.com/api/v3/chat/completions"

	req, err := http.NewRequest(r.Method, openaiURL, bytes.NewBuffer(newReqBody))
	if err != nil {
		// 如果创建请求失败，返回500 Internal Server Error
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		log.Printf("Error creating new request: %v", err)
		return
	}

	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	req.Header.Set("Authorization", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// 如果执行请求失败，返回500 Internal Server Error
		http.Error(w, "Failed to execute request", http.StatusInternalServerError)
		log.Printf("Error executing request: %v", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		log.Printf("Error reading response body: %v", err)
		return
	}

	log.Printf("Response status: %v", resp.Status)
	log.Printf("Response body: %s", respBody)

	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request path: %s", r.URL.Path)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	models := []string{
		"ep-20250126174211-22h6d",
		"gpt-3.5-turbo",
		"gpt-4.0",
	}

	modelResp := ModelResponse{Models: models}
	respBody, err := json.Marshal(modelResp)
	if err != nil {
		http.Error(w, "Failed to create response body", http.StatusInternalServerError)
		log.Printf("Error creating response body: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

func setDefaultParams(req *OpenAIRequest) {
	if req.Temperature == nil {
		defaultTemp := 1.0
		req.Temperature = &defaultTemp
	}

	if req.TopP == nil {
		defaultTopP := 0.7
		req.TopP = &defaultTopP
	}
}

func processMessages(messages []Message, openaiReq *OpenAIRequest) []string {
	var messageArray []string

	for i, message := range messages {
		var content []interface{}
		if err := json.Unmarshal(message.Content, &content); err == nil {
			allText := true
			var combinedText string
			for _, item := range content {
				itemMap, ok := item.(map[string]interface{})
				if !ok || itemMap["type"].(string) != "text" {
					allText = false
					openaiReq.Model = fixedVersionModel
					log.Printf("Not all content are text, so use image model")
					break
				}
				combinedText += itemMap["text"].(string) + " "
			}
			if allText {
				messages[i].Content = json.RawMessage(strconv.Quote(combinedText))
				openaiReq.Model = fixedTextModel
				messageArray = append(messageArray, combinedText)
			}
		} else {
			var textContent string
			if err := json.Unmarshal(message.Content, &textContent); err == nil {
				messages[i].Content = json.RawMessage(strconv.Quote(textContent))
				if len(textContent) > 15000 {
					openaiReq.Model = fixedTextLongModel
					log.Printf("Text is too long(%d), so use long text model", len(textContent))
				} else {
					openaiReq.Model = fixedTextModel
					log.Printf("Text is too short(%d), so use text model", len(textContent))
				}
			} else {
				log.Printf("Failed to parse content as text or slice: %v", err)
				openaiReq.Model = fixedVersionModel
			}
		}
	}

	return messageArray
}

func main() {
	http.HandleFunc("/chat/completions", handler)
	http.HandleFunc("/models", modelsHandler)
	http.HandleFunc("/v1/chat/completions", handler)
	http.HandleFunc("/v1/models", modelsHandler)
	log.Println("Server started on :8280")
	log.Fatal(http.ListenAndServe(":8280", nil))
}
