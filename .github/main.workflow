workflow "Build" {
  on = "push"
  resolves = "Static Analysis"
}

action "Static Analysis" {
  uses = "./.github/static-analysis/"
  runs = "make"
  args = "static-analysis"
}
