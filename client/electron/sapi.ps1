# sapi.ps1 —— YHagent 桌宠单次听写脚本（PowerShell 5.1 内置 System.Speech，行为规范参考 Coopanion）
#
# 协议：stdout 逐行 UTF-8 JSON（ConvertTo-Json 会把非 ASCII 转义为 \uXXXX，无代码页问题）：
#   {"type":"ready"}
#   {"type":"partial","text":"..."}                       实时假设（未定稿，持续覆盖）
#   {"type":"final","text":"...","confidence":0.87}       一句定稿
#   {"type":"done","reason":"completed|timeout|rejected|cancelled|error","detail":"..."}
#
# 生命周期：一次 Start 说一句话（RecognizeAsync Single 模式，句尾静音即定稿），
# 输出 done 后进程自然退出；Go 侧若仍按住 PTT 会重新拉起听下一句。
$ErrorActionPreference = 'Stop'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

Add-Type -AssemblyName System.Speech

# 事件回调运行在识别线程上，不能直接写主输出管道；
# 经线程安全队列投递，主循环轮询取出（ConcurrentQueue 模式）
$queue = New-Object System.Collections.Concurrent.ConcurrentQueue[object]
$state = @{ done = $false }

function Emit($obj) { $obj | ConvertTo-Json -Compress }

$rec = New-Object System.Speech.Recognition.SpeechRecognitionEngine
$rec.InitialSilenceTimeout = [TimeSpan]::FromSeconds(8)   # 起手静音超时（按住后 8s 不说话则 done/timeout）
$rec.BabbleTimeout = [TimeSpan]::FromSeconds(30)           # 连续语音上限，防挂死
$rec.SetInputToDefaultAudioDevice()
$rec.LoadGrammar((New-Object System.Speech.Recognition.DictationGrammar))

$rec.Add_SpeechHypothesized({ param($s, $e)
    $queue.Enqueue(@{ type = 'partial'; text = [string]$e.Result.Text }) })

$rec.Add_SpeechRecognized({ param($s, $e)
    $queue.Enqueue(@{ type = 'final'; text = [string]$e.Result.Text; confidence = [double]$e.Result.Confidence }) })

$rec.Add_SpeechRecognitionRejected({ param($s, $e)
    $queue.Enqueue(@{ type = 'final'; text = ''; confidence = 0.0 }) })

$rec.Add_RecognizeCompleted({ param($s, $e)
    if ($e.Error) {
        $queue.Enqueue(@{ type = 'done'; reason = 'error'; detail = [string]$e.Error.Message })
    } elseif ($e.Cancelled) {
        $queue.Enqueue(@{ type = 'done'; reason = 'cancelled' })
    } elseif ($null -eq $e.Result) {
        $queue.Enqueue(@{ type = 'done'; reason = 'timeout' })
    } else {
        $queue.Enqueue(@{ type = 'done'; reason = 'completed' })
    }
    $state.done = $true })

Emit @{ type = 'ready' }
$rec.RecognizeAsync([System.Speech.Recognition.RecognizeMode]::Single)

while (-not $state.done) {
    $o = $null
    while ($queue.TryDequeue([ref]$o)) { Emit $o }
    Start-Sleep -Milliseconds 40
}
$o = $null
while ($queue.TryDequeue([ref]$o)) { Emit $o }
$rec.Dispose()
