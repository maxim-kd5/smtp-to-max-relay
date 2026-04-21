param(
    [string]$SmtpHost = "127.0.0.1",
    [int]$SmtpPort = 2525,
    [string]$From = "smtp-test@local.dev",
    [string]$To = "chatid-73211480961715@relay.local",
    [string]$Subject = "SMTP relay test for negative chat_id",
    [string]$Body = "Test message from PowerShell SMTP client."
)

$ErrorActionPreference = "Stop"

try {
    $smtp = New-Object Net.Mail.SmtpClient($SmtpHost, $SmtpPort)
    $msg = New-Object Net.Mail.MailMessage
    $msg.From = $From
    $msg.To.Add($To)
    $msg.Subject = $Subject
    $msg.Body = $Body

    $smtp.Send($msg)
    Write-Host "SMTP test sent successfully to $To via $SmtpHost`:$SmtpPort"
}
catch {
    Write-Error "SMTP test failed: $($_.Exception.Message)"
    exit 1
}
finally {
    if ($null -ne $msg) { $msg.Dispose() }
    if ($null -ne $smtp) { $smtp.Dispose() }
}
