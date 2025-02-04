package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

// 长文本模型
var fixedTextLongModel = os.Getenv("FIXED_TEXT_LONG_MODEL")

// 文本模型
var fixedTextModel = os.Getenv("FIXED_TEXT_MODEL")

// 图像模型
var fixedVersionModel = os.Getenv("FIXED_VERSION_MODEL")

// 设置API密钥
var apiKey = os.Getenv("API_KEY")

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
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("请求发送失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
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

	if apiKey == "" {
		// 如果API密钥未设置，返回500 Internal Server Error
		http.Error(w, "API key not set", http.StatusInternalServerError)
		log.Println("Error: API key not set")
		return
	}

	// 创建HTTP客户端
	client := &http.Client{}

	// 读取请求体
	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		// 如果读取请求体失败，返回400 Bad Request
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		log.Printf("Error reading request body: %v", err)
		return
	}
	// log.Printf("Request body: %s", reqBody)

	// 创建一个新的字节缓冲区，用于存储处理后的请求体
	var newReqBodyBuf *bytes.Buffer

	// 如果请求体为空，返回400 Bad Request
	if len(reqBody) == 0 {
		log.Println("Error: Empty request body")
		http.Error(w, "Empty request body", http.StatusBadRequest)
		return
	} else {
		// 解析请求体
		var openaiReq OpenAIRequest
		if err := json.Unmarshal(reqBody, &openaiReq); err != nil {
			// 如果解析请求体失败，返回400 Bad Request
			http.Error(w, "Failed to parse request body", http.StatusBadRequest)
			log.Printf("Error parsing request body: %v", err)
			return
		}

		// 记录解析后的请求信息
		log.Printf("Parsed request: %+v", openaiReq)
		// 如果 Temperature 未设置，则设置为默认值 1.0
		if openaiReq.Temperature == nil {
			defaultTemp := 1.0
			openaiReq.Temperature = &defaultTemp
		}
		// 如果 TopP 未设置，则设置为默认值 0.7
		if openaiReq.TopP == nil {
			defaultTopP := 0.7
			openaiReq.Temperature = &defaultTopP
		}

		var messageArray []string

		// 遍历 messages 列表并处理每个 Message 的 content 字段
		for i, message := range openaiReq.Messages {
			// 声明一个空的 interface{} 切片，用于存储解析后的 content 字段
			var content []interface{}
			// 尝试将 message.Content 解析为 interface{} 切片
			if err := json.Unmarshal(message.Content, &content); err == nil {
				// 假设所有的 content 都是文本类型
				allText := true
				// 声明一个空字符串，用于存储合并后的文本
				var combinedText string
				// 遍历 content 切片
				for _, item := range content {
					// 将 item 转换为 map[string]interface{} 类型
					itemMap, ok := item.(map[string]interface{})
					// 如果转换失败或者 item 的 type 字段不是 "text"，则说明不是所有的 content 都是文本类型
					if !ok || itemMap["type"].(string) != "text" {
						allText = false
						// 如果不是所有的 content 都是文本类型，使用 fixedVersionModel
						openaiReq.Model = fixedVersionModel
						log.Printf("Not all content are text, so use image model")
						break
					}
					// 将 item 的 text 字段添加到 combinedText 中
					combinedText += itemMap["text"].(string) + " "
				}
				// 如果所有的 content 都是文本类型，则将 combinedText 转换为 json.RawMessage 类型并赋值给 message.Content
				if allText {
					// 使用 strconv.Quote 确保字符串合法
					openaiReq.Messages[i].Content = json.RawMessage(strconv.Quote(combinedText))
					openaiReq.Model = fixedTextModel
					messageArray = append(messageArray, combinedText)
				}
			} else {
				// 将 content 解析为 string，如果解析成功则假定 content 是纯文本
				var textContent string
				if err := json.Unmarshal(message.Content, &textContent); err == nil {
					openaiReq.Messages[i].Content = json.RawMessage(strconv.Quote(textContent))
					// 使用不同模型
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

		num, err := GetTotalTokens(fixedTextLongModel, messageArray)
		if err != nil {
			if num > 25000 && openaiReq.Model != fixedVersionModel {
				openaiReq.Model = fixedTextLongModel
			}
		}
		log.Printf("use model: %s, input text: %d", openaiReq.Model, num)

		// 将处理后的请求体转换为字节数组
		newReqBody, err := json.Marshal(openaiReq)
		if err != nil {
			// 如果转换失败，返回500 Internal Server Error
			http.Error(w, "Failed to create new request body", http.StatusInternalServerError)
			log.Printf("Error creating new request body: %v", err)
			return
		}
		// log.Printf("New request body: %s", newReqBody)
		// 创建一个新的字节缓冲区，用于存储处理后的请求体
		newReqBodyBuf = bytes.NewBuffer(newReqBody)
	}

	// 构建实际的OpenAI API URL
	openaiURL := "https://ark.cn-beijing.volces.com/api/v3/chat/completions"

	// 创建一个新的HTTP请求
	req, err := http.NewRequest(r.Method, openaiURL, newReqBodyBuf)
	if err != nil {
		// 如果创建请求失败，返回500 Internal Server Error
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		log.Printf("Error creating new request: %v", err)
		return
	}

	// 设置请求头
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	// 使用鉴权apikey，替换掉用户传递的apikey
	req.Header.Set("Authorization", "Bearer "+apiKey)

	log.Printf("Sending request to OpenAI API: %v", req)

	// 发送请求并获取响应
	resp, err := client.Do(req)
	if err != nil {
		// 如果执行请求失败，返回500 Internal Server Error
		http.Error(w, "Failed to execute request", http.StatusInternalServerError)
		log.Printf("Error executing request: %v", err)
		return
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// 如果读取响应体失败，返回500 Internal Server Error
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		log.Printf("Error reading response body: %v", err)
		return
	}

	log.Printf("Response status: %v", resp.Status)
	log.Printf("Response body: %s", respBody)

	// 将响应状态码和响应体返回给客户端
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request path: %s", r.URL.Path)

	// 设置CORS头信息
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// 提前处理OPTIONS请求
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

func main() {
	http.HandleFunc("/chat/completions", handler)
	http.HandleFunc("/models", modelsHandler)
	log.Println("Server started on :8280")
	log.Fatal(http.ListenAndServe(":8280", nil))
}
