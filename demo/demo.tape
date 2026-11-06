# bark demo: Linus reviews a staged change.
# Run via demo/record.sh (it builds the sample repo and the demo binary).

Output assets/demo.gif

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1500
Set Height 720
Set Padding 20
Set TypingSpeed 60ms

# Hidden setup: isolated HOME/XDG, demo binary on PATH, offline mock LLM,
# neutral prompt. Nothing here is shown in the GIF.
Hide
Type `export HOME="$PWD/demo/.tmp/home" XDG_CONFIG_HOME="$PWD/demo/.tmp/xdg" PATH="$PWD/demo/.tmp/bin:$PATH"` Enter
Type `export BARK_DEMO_MOCK=1 GIT_PAGER=cat LANG=en_US.UTF-8 GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null` Enter
Type `cd demo/.tmp/sample-repo && export PS1='\[\e[1;35m\]sample-repo\[\e[0m\] \[\e[32m\]❯\[\e[0m\] '` Enter
Type "clear" Enter
Sleep 500ms
Show

# 1. A staged change.
Sleep 600ms
Type "git diff --staged --stat"
Sleep 300ms
Enter
Sleep 1.8s

# 2. Ask Linus for a review.
Type "bark review --staged --as linus -k"
Sleep 500ms
Enter

# 3. The review streams in; wait for the "Reviewed by" status line rather than sleeping.
Wait+Screen@25s /Reviewed by Linus/

# 4. Hold on the finished review.
Sleep 4s
