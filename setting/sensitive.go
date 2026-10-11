package setting

import "strings"

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"窃取 API 密钥",
	"窃取API密钥",
	"盗用 API 密钥",
	"盗用API密钥",
	"提取 OAuth 令牌",
	"提取OAuth令牌",
	"批量轮换账号规避检测",
	"批量轮换密钥规避检测",
	"批量注册账号规避检测",
	"绕过计费限制",
	"绕过额度限制",
	"steal api key",
	"exfiltrate api key",
	"rotate accounts to evade detection",
	"rotate keys to evade detection",
	"bypass billing limits",
}

func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

func SensitiveWordsFromString(s string) {
	SensitiveWords = []string{}
	sw := strings.SplitSeq(s, "\n")
	for w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveWords = append(SensitiveWords, w)
		}
	}
}

func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
