Output docs/assets/interactive-resume.gif

Set Shell bash
Set Width 1200
Set Height 720
Set FontSize 16
Set TypingSpeed 55ms
Set Theme "GitHub Dark"
Set Padding 24
Set Margin 24
Set MarginFill "#11111b"
Set WindowBar Colorful
Set BorderRadius 10
Set CursorBlink false

Hide
Type "go build -o /tmp/agentx-vhs-demo ./docs/demo"
Enter
Type `export PATH="$PWD/docs/demo/bin:/usr/bin:/bin"`
Enter
Type `alias ax="/tmp/agentx-vhs-demo"`
Enter
Type `export PS1="$ "`
Enter
Type `printf '\033[0;38;2;230;237;243m'`
Enter
Type "clear"
Enter
Show

Type "ax"
Sleep 400ms
Enter
Wait+Screen /Choose a recent session/
Sleep 1.4s
Down
Sleep 900ms
Down
Sleep 1.8s

Hide
Ctrl+C
Sleep 200ms
Type "/bin/rm -f /tmp/agentx-vhs-demo"
Enter
