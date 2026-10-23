# Copies a compiled SynthDef (an SCgf v2 file holding one def) under a new
# name, renaming some of its parameters on the way:
#
#   ruby synthdef-rename.rb IN OUT NEW_NAME [old=new ...]
#
# Only the name strings change; the graph is copied byte for byte.
src, dst, new_name, *pairs = ARGV
abort "usage: synthdef-rename.rb IN OUT NEW_NAME [old=new ...]" unless new_name
renames = pairs.to_h { |p| p.split("=", 2) }

b = File.binread(src)
abort "#{src}: not a single-def SCgf v2 file" unless b[0, 4] == "SCgf" && b[4, 6].unpack("l>s>") == [2, 1]

def pstr(s)
  s = s.b
  abort "name too long: #{s}" if s.bytesize > 255
  [s.bytesize].pack("C") + s
end

out = b[0, 10].b
i = 10
i += 1 + b.getbyte(i)                                  # the def's name
out << pstr(new_name)
start = i
nconsts = b[i, 4].unpack1("l>"); i += 4 + 4 * nconsts
nparams = b[i, 4].unpack1("l>"); i += 4 + 4 * nparams
nnames = b[i, 4].unpack1("l>"); i += 4
out << b[start...i]
seen = []
nnames.times do
  len = b.getbyte(i)
  name = b[i + 1, len]
  seen << name
  out << pstr(renames.fetch(name, name)) << b[i + 1 + len, 4]
  i += 1 + len + 4
end
missing = renames.keys - seen
abort "#{src} has no parameter #{missing.join(', ')}" unless missing.empty?
out << b[i..]
File.binwrite(dst, out)
