if vim.g.loaded_agentws then
  return
end
vim.g.loaded_agentws = 1

vim.api.nvim_create_user_command("AgentwsDiff", function(o)
  require("agentws").diff(o.args)
end, {
  nargs = "?",
  complete = function()
    return { "last_turn", "uncommitted", "branch" }
  end,
  desc = "agentws: open the session's review scope in diffview",
})

vim.api.nvim_create_user_command("AgentwsComment", function(o)
  require("agentws").comment(o.line1, o.line2, o.args)
end, {
  nargs = "*",
  range = true,
  desc = "agentws: add the lines as a draft review comment",
})
