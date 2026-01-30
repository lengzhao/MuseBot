package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
	
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yincongcyincong/MuseBot/conf"
	"github.com/yincongcyincong/MuseBot/logger"
	"github.com/yincongcyincong/MuseBot/metrics"
	"github.com/yincongcyincong/MuseBot/utils"
)

var (
	FilterPath = map[string]bool{
		"/pong": true,
	}
)

type HTTPServer struct {
	Addr string
}

func InitHTTP() {
	initImg()
	pprofServer := NewHTTPServer(fmt.Sprintf("%s", conf.BaseConfInfo.HTTPHost))
	pprofServer.Start()
}

// NewHTTPServer create http server, listen 36060 port.
func NewHTTPServer(addr string) *HTTPServer {
	if addr == "" {
		addr = ":36060"
	}
	return &HTTPServer{
		Addr: addr,
	}
}

// Start pprof server
func (p *HTTPServer) Start() {
	go func() {
		logger.Info("Starting pprof server on", "addr", p.Addr)
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		
		mux.HandleFunc("/user/token/add", AddUserToken)
		
		mux.HandleFunc("/conf/update", UpdateConf)
		mux.HandleFunc("/conf/get", GetConf)
		mux.HandleFunc("/command/get", GetCommand)
		mux.HandleFunc("/restart", Restart)
		mux.HandleFunc("/stop", Stop)
		mux.HandleFunc("/log", Log)
		
		mux.HandleFunc("/mcp/get", GetMCPConf)
		mux.HandleFunc("/mcp/update", UpdateMCPConf)
		mux.HandleFunc("/mcp/disable", DisableMCPConf)
		mux.HandleFunc("/mcp/delete", DeleteMCPConf)
		mux.HandleFunc("/mcp/sync", SyncMCPConf)
		
		mux.HandleFunc("/user/list", GetUsers)
		mux.HandleFunc("/user/insert/record", InsertUserRecords)
		mux.HandleFunc("/record/list", GetRecords)
		
		mux.HandleFunc("/rag/list", GetRagFile)
		mux.HandleFunc("/rag/delete", DeleteRagFile)
		mux.HandleFunc("/rag/create", CreateRagFile)
		mux.HandleFunc("/rag/get", GetRagFileContent)
		mux.HandleFunc("/rag/clear", ClearAllVectorData)
		
		mux.HandleFunc("/pong", PongHandler)
		mux.HandleFunc("/dashboard", DashboardHandler)
		
		// 静态文件服务 - 提供聊天页面
		mux.HandleFunc("/chat", serveChatPage)
		mux.HandleFunc("/", serveChatPage) // 根路径也提供聊天页面
		
		mux.HandleFunc("/communicate", Communicate)
		mux.HandleFunc("/com/wechat", ComWechatComm)
		mux.HandleFunc("/wechat", WechatComm)
		mux.HandleFunc("/qq", QQBotComm)
		mux.HandleFunc("/onebot", OneBot)
		
		mux.HandleFunc("/cron/create", CreateCron)
		mux.HandleFunc("/cron/update", UpdateCron)
		mux.HandleFunc("/cron/update_status", UpdateCronStatus)
		mux.HandleFunc("/cron/delete", DeleteCron)
		mux.HandleFunc("/cron/list", GetCrons)
		
		mux.HandleFunc("/image", imageHandler)
		
		wrappedMux := WithRequestContext(mux)
		
		var err error
		if conf.BaseConfInfo.CrtFile == "" || conf.BaseConfInfo.KeyFile == "" {
			err = http.ListenAndServe(p.Addr, wrappedMux)
		} else {
			err = runTLSServer(wrappedMux)
		}
		if err != nil {
			logger.Fatal("pprof server failed", "err", err)
		}
	}()
}

func runTLSServer(wrappedMux http.Handler) error {
	caCert, err := os.ReadFile(conf.BaseConfInfo.CaFile)
	if err != nil {
		return err
	}
	
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	
	cert, err := tls.LoadX509KeyPair(conf.BaseConfInfo.CrtFile, conf.BaseConfInfo.KeyFile)
	if err != nil {
		return err
	}
	
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
	
	server := &http.Server{
		Addr:      fmt.Sprintf("%s", conf.BaseConfInfo.HTTPHost),
		TLSConfig: tlsConfig,
		Handler:   wrappedMux,
	}
	
	err = server.ListenAndServeTLS("", "")
	if err != nil {
		return err
	}
	
	return nil
}

func WithRequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		
		isSSE := r.Header.Get("Accept") == "text/event-stream"
		
		var cancel context.CancelFunc
		if !isSSE {
			ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
		}
		
		logID := r.Header.Get("LogId")
		if logID == "" {
			logID = uuid.New().String()
		}
		ctx = context.WithValue(ctx, "log_id", logID)
		
		if conf.BaseConfInfo.BotName != "" {
			ctx = context.WithValue(ctx, "bot_name", conf.BaseConfInfo.BotName)
		}
		
		ctx = context.WithValue(ctx, "start_time", time.Now())
		
		r = r.WithContext(ctx)
		
		if !FilterPath[r.URL.Path] {
			logger.InfoCtx(ctx, "request start", "path", r.URL.Path)
		}
		
		next.ServeHTTP(w, r)
		
		metrics.HTTPRequestCount.WithLabelValues(r.URL.Path).Inc()
	})
}

// serveChatPage 提供聊天页面
func serveChatPage(w http.ResponseWriter, r *http.Request) {
	// 只对根路径和 /chat 路径提供聊天页面
	if r.URL.Path != "/" && r.URL.Path != "/chat" {
		http.NotFound(w, r)
		return
	}
	
	chatHTMLPath := utils.GetAbsPath("static/chat.html")
	
	// 检查文件是否存在
	if _, err := os.Stat(chatHTMLPath); os.IsNotExist(err) {
		// 如果文件不存在，返回简单的HTML
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>MuseBot 聊天</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .chat-container { border: 1px solid #ddd; border-radius: 10px; padding: 20px; }
        .messages { height: 400px; overflow-y: auto; border: 1px solid #eee; padding: 10px; margin-bottom: 10px; }
        .input-area { display: flex; gap: 10px; }
        input { flex: 1; padding: 10px; border: 1px solid #ddd; border-radius: 5px; }
        button { padding: 10px 20px; background: #667eea; color: white; border: none; border-radius: 5px; cursor: pointer; }
        button:hover { background: #5568d3; }
        .message { margin: 10px 0; padding: 10px; border-radius: 5px; }
        .user { background: #e3f2fd; text-align: right; }
        .assistant { background: #f5f5f5; }
    </style>
</head>
<body>
    <div class="chat-container">
        <h1>MuseBot 聊天</h1>
        <div class="messages" id="messages"></div>
        <div class="input-area">
            <input type="text" id="input" placeholder="输入消息..." onkeypress="if(event.key==='Enter') send()">
            <button onclick="send()">发送</button>
        </div>
    </div>
    <script>
        const host = window.location.hostname;
        const port = window.location.port || '36060';
        const apiUrl = "http://" + host + ":" + port + "/communicate";
        
        function addMessage(text, isUser) {
            const div = document.createElement('div');
            div.className = 'message ' + (isUser ? 'user' : 'assistant');
            div.textContent = text;
            document.getElementById('messages').appendChild(div);
            document.getElementById('messages').scrollTop = document.getElementById('messages').scrollHeight;
        }
        
        async function send() {
            const input = document.getElementById('input');
            const message = input.value.trim();
            if (!message) return;
            
            addMessage(message, true);
            input.value = '';
            
            const assistantDiv = document.createElement('div');
            assistantDiv.className = 'message assistant';
            document.getElementById('messages').appendChild(assistantDiv);
            
            // 使用 sessionStorage 保存 user_id，确保同一浏览器会话使用相同的 user_id
            let userId = sessionStorage.getItem('muse_bot_user_id');
            if (!userId) {
                userId = "web_user_" + Date.now();
                sessionStorage.setItem('muse_bot_user_id', userId);
            }
            
            try {
                const response = await fetch(apiUrl + "?prompt=" + encodeURIComponent(message) + "&user_id=" + encodeURIComponent(userId), {
                    method: 'POST'
                });
                
                const reader = response.body.getReader();
                const decoder = new TextDecoder();
                let content = '';
                
                while (true) {
                    const {done, value} = await reader.read();
                    if (done) break;
                    content += decoder.decode(value, {stream: true});
                    assistantDiv.textContent = content;
                    document.getElementById('messages').scrollTop = document.getElementById('messages').scrollHeight;
                }
            } catch (error) {
                assistantDiv.textContent = '错误: ' + error.message;
            }
        }
    </script>
</body>
</html>`)
		return
	}
	
	// 读取并返回HTML文件
	http.ServeFile(w, r, chatHTMLPath)
}
