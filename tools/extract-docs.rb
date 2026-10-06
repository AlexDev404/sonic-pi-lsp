#!/usr/bin/env ruby
# frozen_string_literal: true
#
# Extracts everything the language server knows about Sonic Pi from a Sonic Pi
# source checkout's own doc system, and writes it as one JSON file.
#
#   ruby tools/extract-docs.rb /path/to/sonic-pi [data/sonic-pi.json]
#
# Docs stay Markdown (the doc system's own format), which is what LSP hovers
# and completion details render. The output is committed, so building or using
# the server never needs Ruby or a Sonic Pi checkout; rerun this when Sonic Pi's
# docs change.
#
# Reads:
#   app/server/ruby/lib/sonicpi/lang/*.rb        functions (`doc name: ...`)
#   app/server/ruby/lib/sonicpi/synths/synthinfo.rb  synths, FX, samples
#   app/server/ruby/lib/sonicpi/{chord,scale,tuning}.rb

require "json"

sp_root = ARGV[0] || ENV["SONIC_PI_ROOT"]
abort "usage: ruby tools/extract-docs.rb /path/to/sonic-pi [out.json]" unless sp_root
sp_root = File.expand_path(sp_root)
out = File.expand_path(ARGV[1] || File.join(__dir__, "..", "data", "sonic-pi.json"))
server = File.join(sp_root, "app/server/ruby")
abort "not a Sonic Pi checkout: #{sp_root}" unless File.exist?(File.join(server, "core.rb"))

require File.join(server, "core.rb")
require File.join(server, "paths")
%w[synths/synthinfo util runtime lang/core lang/sound lang/midi lang/western_theory note chord scale tuning].each do |f|
  require File.join(server, "lib/sonicpi", f)
end

# Integral floats as integers, so `1.0` reads `1` in hovers.
num = ->(n) { n.is_a?(Float) && n.finite? && n == n.to_i ? n.to_i : n }

plain = lambda do |v|
  case v
  when Numeric then num.call(v)
  when true, false, nil then v
  when Symbol then v.inspect
  else v.to_s
  end
end

# An opt as the server needs it: its doc, default, and the bounds Sonic Pi
# validates it against at run time (not a GUI slider's range).
opt_entry = lambda do |info, default|
  o = { "doc" => info[:doc].to_s.strip }
  d = plain.call(default.nil? ? info[:default] : default)
  o["default"] = d unless d.nil?
  b = info[:bounds] || {}
  if b[:options]
    o["options"] = b[:options].map { |x| plain.call(x) }
  else
    if b.key?(:min)
      o["min"] = num.call(b[:min])
      o["minIncl"] = b[:min_incl] != false
    end
    if b.key?(:max)
      o["max"] = num.call(b[:max])
      o["maxIncl"] = b[:max_incl] != false
    end
  end
  cons = info[:constraints] || []
  o["constraints"] = cons unless cons.empty?
  o["slidable"] = true if info[:slidable]
  o["bpmScale"] = true if info[:bpm_scale]
  o["midi"] = true if info[:midi]
  o
end

# Every opt a synth or FX takes, documented: arg_info, plus the `_slide`,
# `_slide_shape` and `_slide_curve` variants that only arg_defaults and
# info list.
all_opts = lambda do |v|
  defaults = v.arg_defaults
  opts = {}
  v.arg_info.each { |ak, info| opts[ak.to_s] = opt_entry.call(info, defaults[ak]) }
  (defaults.keys - v.arg_info.keys).each do |ak|
    i = (v.info[ak] rescue nil) || {}
    vals = i[:validations] || []
    bounds = vals.map { |x| x[2] }.compact.reduce({}) { |a, b| a.merge(b) }
    info = { doc: i[:doc], bounds: bounds, constraints: vals.map { |x| x[1] }, bpm_scale: i[:bpm_scale] }
    opts[ak.to_s] = opt_entry.call(info, defaults[ak])
  end
  opts
end

summary_of = ->(doc) { t = doc.to_s.gsub(/\s+/, " ").strip; t[/\A.+?[.!?](?=\s|\z)/] || t }

instruments = lambda do |want_fx|
  res = {}
  SonicPi::Synths::SynthInfo.get_all.each do |k, v|
    next if v.is_a?(SonicPi::Synths::StudioInfo)
    is_fx = v.is_a?(SonicPi::Synths::FXInfo)
    next unless is_fx == want_fx
    next if is_fx && k.to_s.include?("replace_")
    key = is_fx ? k.to_s.sub(/\Afx_/, "") : k.to_s
    res[key] = { "title" => v.name.to_s, "summary" => summary_of.call(v.doc), "doc" => v.doc.to_s.strip, "opts" => all_opts.call(v) }
  end
  res.sort.to_h
end

synths = instruments.call(false)
fx = instruments.call(true)

# Functions. The doc system's table is shared by every Lang module.
functions = {}
SonicPi::Lang::Core.docs.each do |name, v|
  next if v[:hide]
  summary = (v[:summary] || v[:name]).to_s.dup
  summary[0] = summary[0].upcase unless summary.empty?
  opts = v[:opts].is_a?(Hash) && !v[:opts].key?(:your_key) ? v[:opts].transform_keys(&:to_s).transform_values { |d| d.to_s.strip } : {}
  f = {
    "summary" => summary,
    "args" => (v[:args] || []).map { |n, t| { "name" => n.to_s, "type" => t.to_s } },
    "opts" => opts,
    "doc" => v[:doc].to_s.strip,
    "examples" => (v[:examples] || []).map { |e| e.to_s.strip }.reject(&:empty?),
  }
  f["usage"] = v[:usage_example].to_s.strip unless v[:usage_example].to_s.strip.empty?
  f["introduced"] = v[:introduced].to_s if v[:introduced]
  f["acceptsBlock"] = true if v[:accepts_block]
  f["requiresBlock"] = true if v[:requires_block]
  f["returns"] = v[:returns].to_s if v[:returns]
  f["argKinds"] = v[:arg_kinds].map { |k| k&.to_s } if v[:arg_kinds] && !v[:arg_kinds].empty?
  functions[name.to_s] = f
end
functions = functions.sort.to_h

# `sample` takes the stereo player's opts plus the ones the sampler handles itself.
sample_opts = all_opts.call(SonicPi::Synths::SynthInfo.get_all[:stereo_player])
(functions.dig("sample", "opts") || {}).each { |k, d| sample_opts[k] ||= { "doc" => d } }

samples = {}
sample_groups = SonicPi::Synths::SynthInfo.grouped_samples.map do |_, g|
  g[:samples].each { |s| samples[s.to_s] = g[:desc].to_s }
  { "title" => g[:desc].to_s, "samples" => g[:samples].map(&:to_s) }
end

intervals = lambda do |names, build|
  res = {}
  names.each do |k|
    next if res.key?(k.to_s)
    offs = (build.call(k).to_a rescue nil)
    res[k.to_s] = offs.map { |x| num.call(x.round(3)) } if offs && !offs.empty?
  end
  res
end
chords = intervals.call(SonicPi::Chord::CHORD_LOOKUP.keys, ->(k) { SonicPi::Chord.new(0, k) })
scales = intervals.call(SonicPi::Scale::SCALE.keys, ->(k) { SonicPi::Scale.new(0, k) })

data = {
  "sonicPiVersion" => File.read(File.join(sp_root, "VERSION")).strip,
  "functions" => functions,
  "synths" => synths,
  "fx" => fx,
  "sampleOpts" => sample_opts,
  "samples" => samples,
  "sampleGroups" => sample_groups,
  "chords" => chords,
  "scales" => scales,
  "tunings" => (["equal"] + SonicPi::Tuning.new.tunings.keys.map(&:to_s)).uniq,
  "randomSources" => %w[white pink light_pink dark_pink perlin],
}

require "fileutils"
FileUtils.mkdir_p File.dirname(out)
File.write(out, JSON.pretty_generate(data) + "\n")
puts "Sonic Pi #{data['sonicPiVersion']}: #{functions.size} functions, #{synths.size} synths, #{fx.size} fx, " \
     "#{samples.size} samples, #{chords.size} chords, #{scales.size} scales -> #{out}"
