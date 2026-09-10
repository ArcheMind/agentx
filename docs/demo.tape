Output docs/assets/interactive-resume.gif

Set Shell bash
Set Width 1200
Set Height 720
Set FontSize 22
Set TypingSpeed 55ms
Set Theme "Catppuccin Mocha"
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
Type "clear"
Enter
Show

Type "ax"
Sleep 400ms
Enter
Wait+Line /Selection \(or q to cancel\):/
Sleep 1s
Type "2"
Sleep 300ms
Enter
Wait+Line /Selection \(or q to cancel\):/
Sleep 1.2s
Type "1"
Sleep 300ms
Enter
Wait+Line /Selection \(or q to cancel\):/
Sleep 1s
Type "1"
Sleep 300ms
Enter
Wait+Line /Selection \(or q to cancel\):/
Sleep 1.2s
Type "2"
Sleep 300ms
Enter
Wait+Screen /Session handed off/
Sleep 2.5s

Hide
Type "/bin/rm -f /tmp/agentx-vhs-demo"
Enter
