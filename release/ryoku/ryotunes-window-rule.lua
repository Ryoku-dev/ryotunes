-- Ryoku-native window policy for the Gio client.
hl.window_rule({
    name   = "float-ryotunes",
    match  = { class = "^ryotunes$", title = "^Ryotunes$" },
    float  = true,
    size   = { 1760, 1000 },
    center = true,
})

hl.window_rule({
    name   = "float-ryotunes-mini",
    match  = { class = "^ryotunes$", title = "^Ryotunes Mini$" },
    float  = true,
    size   = { 640, 200 },
    center = true,
})
