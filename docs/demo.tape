Output docs/assets/interactive-resume.gif

Set Shell bash
Set Width 1200
Set Height 720
Set FontSize 16
Set TypingSpeed 25ms
Set Theme "GitHub Dark"
Set Padding 24
Set Margin 24
Set MarginFill "#11111b"
Set WindowBar Colorful
Set BorderRadius 10
Set CursorBlink false

Hide
Type "make demo-build"
Enter
Type `export PATH="$PWD/docs/demo/bin:/usr/bin:/bin"`
Enter
Type `export SM_SUBAGENT_HOME="$PWD/docs/demo/home"`
Enter
Type `alias sm="/tmp/sm-vhs-demo"`
Enter
Type `export PS1="$ "`
Enter
Type `printf '\033[0;38;2;230;237;243m'`
Enter
Type "clear"
Enter
Show

Type "sm"
Sleep 400ms
Enter
Wait+Screen /Recent sessions/
Wait+Screen /Resume session as:/
Wait+Screen /Choose a model:/
Sleep 1.5s

Hide
Ctrl+C
Sleep 200ms
Type "/bin/rm -f /tmp/sm-vhs-demo"
Enter
