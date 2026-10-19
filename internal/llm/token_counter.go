package llm

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

//go:embed o200k_base.tiktoken
var bpeData embed.FS

func init() {
	tiktoken.SetBpeLoader(&embedLoader{})
}

// embedLoader 从 embed.FS 加载 BPE 文件，零网络依赖。
type embedLoader struct{}

func (l *embedLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	f, err := bpeData.Open("o200k_base.tiktoken")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	contents, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	bpeRanks := make(map[string]int)
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		bpeRanks[string(token)] = rank
	}
	return bpeRanks, nil
}

var (
	tokenEncOnce sync.Once
	tokenEnc     *tiktoken.Tiktoken
	tokenEncErr  error
)

// getTokenEnc 返回 o200k_base 编码器，全局复用。
func getTokenEnc() (*tiktoken.Tiktoken, error) {
	tokenEncOnce.Do(func() {
		tokenEnc, tokenEncErr = tiktoken.GetEncoding("o200k_base")
	})
	return tokenEnc, tokenEncErr
}

// CountTokens 计算一段文本的 token 数。
func CountTokens(text string) (int, error) {
	enc, err := getTokenEnc()
	if err != nil {
		return 0, err
	}
	if text == "" {
		return 0, nil
	}
	return len(enc.Encode(text, nil, nil)), nil
}

// CountMessageTokens 估算结构化消息的 token 数，不统计来源和不透明原生 payload。
// 与 Python _count_msg_tokens 一致：只算 content + tool_calls + tool_call_id + reasoning_content。
func CountMessageTokens(msg Message) (int, error) {
	enc, err := getTokenEnc()
	if err != nil {
		return 0, err
	}

	n := 0
	if msg.Content != "" {
		n += len(enc.Encode(msg.Content, nil, nil))
	}
	if len(msg.ToolCalls) > 0 {
		b, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return 0, err
		}
		n += len(enc.Encode(string(b), nil, nil))
	}
	if msg.ToolCallID != "" {
		n += len(enc.Encode(msg.ToolCallID, nil, nil))
	}
	if msg.ThinkingContent != "" {
		n += len(enc.Encode(msg.ThinkingContent, nil, nil))
	}
	return n, nil
}

// CountMessagesTokens 计算消息列表的总 token 数，并按 role 分组计数。
func CountMessagesTokens(messages []Message) (total int, byRole map[string]int, err error) {
	byRole = map[string]int{}
	for _, msg := range messages {
		n, err := CountMessageTokens(msg)
		if err != nil {
			return 0, nil, err
		}
		total += n
		byRole[msg.Role] += n
	}
	return total, byRole, nil
}
