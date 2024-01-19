package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	log "github.com/sirupsen/logrus"
)

var BalanceAPI = "https://ec2-18-218-197-117.us-east-2.compute.amazonaws.com/balance?address=%s"

var (
	MintCount  atomic.Uint64
	Address    string
	Prefix     string
	Challenge  string
	HexAddress string
	addrs      []string
	HttpClient *http.Client
	Index      atomic.Uint64
)

func init() {
	file, err := os.Open("./wal.csv")
	Index.Store(0)
	if err != nil {
		fmt.Println("无法打开文件:", err)
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		addrs = append(addrs, strings.Split(line, ",")[0])
	}

	// 检查是否有扫描错误
	if err := scanner.Err(); err != nil {
		fmt.Println("扫描文件时发生错误:", err)
		return
	}
	for _, line := range addrs {
		fmt.Println(line)
	}
	Challenge = "72424e4200000000000000000000000000000000000000000000000000000000"
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "15:04:05", FullTimestamp: true})
	Address = addrs[Index.Load()]
	fmt.Println("当前钱包地址:", Address, "索引:", Index.Load())
	Address = strings.ToLower(strings.TrimPrefix(Address, "0x"))
	HexAddress = "0x" + Address
	fmt.Print("请输入难度：")
	_, err = fmt.Scanln(&Prefix)
	if err != nil {
		return
	}
	//HttpClient = SSClient(&http.Client{Timeout: time.Second})
	HttpClient = &http.Client{Timeout: time.Second}
}

func main() {
	url := fmt.Sprintf(BalanceAPI, HexAddress)
	minted := uint64(0)
	//tp := &http.Transport{
	//	DialContext: Dialer.NewConn,
	//}
	client := &http.Client{
		Timeout: time.Millisecond * 20000,
		//Transport: tp,
	}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Println("query balance error", err)
	} else {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("read balance error", err)
		} else {
			var rmap map[string]any
			err := json.Unmarshal(b, &rmap)
			if err != nil {
				fmt.Println("unmarshal balance error", err)
			} else {
				minted = uint64((rmap["balance"]).(float64))
			}
		}
	}
	MintCount.Store(minted)
Mint:
	ctx, c := context.WithCancel(context.Background())
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					makeTx()
				}

			}
		}()
	}
	tick := time.NewTicker(3 * time.Second)
loop:
	for {
		select {
		case <-tick.C:
			mc := MintCount.Load()
			if mc >= 4900 {
				c()
				break loop
			}
			log.WithFields(log.Fields{"MintCount": mc}).Info("Address=", Address)
		}
	}
	Index.Add(1)
	Address = addrs[Index.Load()]
	Address = strings.ToLower(strings.TrimPrefix(Address, "0x"))
	HexAddress = "0x" + Address
	fmt.Println("当前钱包地址:", Address, "索引:", Index.Load())

	MintCount.Store(0)
	goto Mint
}

func sendTX(body string) {
	//tp := &http.Transport{
	//	DialContext: Dialer.NewConn,
	//}
	//client := &http.Client{
	//	Timeout: time.Millisecond * 1000,
	//Transport: tp,
	//}
	var data = strings.NewReader(body)
	req, err := http.NewRequest("POST", "https://ec2-18-217-135-255.us-east-2.compute.amazonaws.com/validate", data)
	if err != nil {
		log.Error(err)
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("origin", "https://bnb.reth.cc")
	req.Header.Set("referer", "https://bnb.reth.cc/")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := HttpClient.Do(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			MintCount.Add(1)
			log.Info("MINT成功")
		} else {
			log.Error(err)
		}
		return
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error(err)
			return
		}
	}(resp.Body)

	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return
	}
	bodyString := string(bodyText)
	containsValidateSuccess := strings.Contains(bodyString, "validate success!")
	if containsValidateSuccess {
		log.Info("MINT成功")
		MintCount.Add(1)
	} else {
		log.WithFields(log.Fields{"错误": err}).Error("MINT错误")
	}

}

func makeTx() {
	randomValue := make([]byte, 32)
	_, err := rand.Read(randomValue)
	if err != nil {
		log.Error(err)
		return
	}

	potentialSolution := hex.EncodeToString(randomValue)
	//fmt.Println("hex address", Address)
	address64 := fmt.Sprintf("%064s", strings.ToLower(Address))
	dataTemps := fmt.Sprintf(`%s%s%s`, potentialSolution, Challenge, address64)

	dataBytes, err := hex.DecodeString(dataTemps)
	if err != nil {
		fmt.Println("oops!")
		log.Error(err)
		return
	}

	hashedSolutionBytes := crypto.Keccak256(dataBytes)
	hashedSolution := fmt.Sprintf("0x%s", hex.EncodeToString(hashedSolutionBytes))

	if strings.HasPrefix(hashedSolution, Prefix) {
		log.WithFields(log.Fields{"Solution": hashedSolution}).Info("找到新ID")
		body := fmt.Sprintf(`{"solution": "0x%s", "challenge": "0x%s", "address": "%s", "difficulty": "%s", "tick": "%s"}`, potentialSolution, Challenge, strings.ToLower(HexAddress), Prefix, "rBNB")
		sendTX(body)
	}
}

type ApiResponse struct {
	Address string `json:"address"`
	Balance int    `json:"balance"`
}

func getBalance(address string) int {
	client := &http.Client{}
	url := "https://ec2-18-217-135-255.us-east-2.compute.amazonaws.com/balance?address=" + address

	for {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Println("创建余额请求失败:", err)
			continue // 继续尝试
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Println("请求余额失败，正在重试:", err)
			continue // 继续尝试
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Println("读取余额响应体失败:", err)
			continue // 继续尝试
		}

		var response ApiResponse
		err = json.Unmarshal(body, &response)
		if err != nil {
			log.Println("解析余额 JSON 失败:", err)
			continue // 继续尝试
		}
		fmt.Println(string(body))
		// 检查响应是否包含预期的字段
		if response.Address == "" {
			log.Println("响应格式不符合预期，继续重试")
			continue // 继续尝试
		}

		return response.Balance
	}
}
