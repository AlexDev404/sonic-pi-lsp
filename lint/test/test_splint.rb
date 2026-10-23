# frozen_string_literal: true

# Tests for the linter: ruby -Ilint/lib lint/test/test_splint.rb
# SONICPI_ROOT picks the Sonic Pi tree (default: external/sonic-pi).

require "minitest/autorun"
require "tmpdir"

$LOAD_PATH.unshift File.expand_path("../lib", __dir__)
require "splint/ast"
require "splint/knowledge"
require "splint/rules"
require "splint/linter"
require "splint/config"
require "splint/report"

ROOT = ENV["SONICPI_ROOT"] || File.expand_path("../../external/sonic-pi", __dir__)

class SplintTest < Minitest::Test
  def k = (@@k ||= Splint::Knowledge.load(ROOT))

  def lint(code, rules: {}, preset: "recommended")
    cfg = Splint::Config.new({ "extends" => preset, "rules" => rules, "use_init" => false }, path: nil)
    Splint::Linter.new(k).lint("t.rb", code, cfg)
  end

  def rules_of(code, **kw) = lint(code, **kw).map(&:rule)

  def assert_flags(rule, code, **kw)
    assert_includes rules_of(code, **kw), rule, "expected #{rule} for:\n#{code}"
  end

  def refute_flags(rule, code, **kw)
    d = lint(code, **kw).select { |x| x.rule == rule }
    assert_empty d, "unexpected #{rule}: #{d.map(&:message).join('; ')}\n#{code}"
  end

  def test_knowledge_comes_from_sonic_pi
    assert k.synth?(:prophet)
    assert k.fx?(:reverb)
    assert k.sample?(:bd_haus)
    assert k.note?(:Eb3)
    refute k.note?(:h4)
    assert k.chord?(:minor7)
    assert k.scale?(:major_pentatonic)
    assert_match(/\A\d+\.\d+/, k.version)
  end

  def test_unknown_names
    assert_flags "unknown-synth", "use_synth :prophett\n"
    assert_flags "unknown-fx", "with_fx :reverbb do\n  play 60\nend\n"
    assert_flags "unknown-sample", "sample :bd_hauss\n"
    assert_flags "unknown-note", "play :h4\n"
    assert_flags "unknown-chord", "play chord(:e3, :minr)\n"
    assert_flags "unknown-scale", "play scale(:c, :majr).choose\n"
    refute_flags "unknown-sample", "sample :bd_haus\nsample \"/tmp\", :foo\n"
    refute_flags "unknown-note", "play :r\nplay :rest\nplay [:c4, :e4]\nplay 60\n"
  end

  def test_suggestions
    d = lint("use_synth :prophett\n").find { |x| x.rule == "unknown-synth" }
    assert_match(/did you mean :prophet/, d.message)
    d = lint("use_synth :reverb\n").find { |x| x.rule == "unknown-synth" }
    assert_match(/is an FX/, d.message)
  end

  def test_opts_follow_the_current_synth
    assert_flags "unknown-opt", "use_synth :beep\nplay 60, cutoff: 80\n"
    refute_flags "unknown-opt", "use_synth :tb303\nplay 60, cutoff: 80, res: 0.9, cutoff_slide: 1\n"
    refute_flags "unknown-opt", "with_synth :tb303 do\n  play 60, cutoff: 80\nend\n"
    assert_flags "unknown-opt", "with_synth :tb303 do\n  play 60, cutoff: 80\nend\nplay 60, cutoff: 70\n"
    refute_flags "unknown-opt", "define :x do\n  play 60, cutoff: 80\nend\n"
    assert_flags "unknown-opt", "define :x do\n  play 60, cutof: 80\nend\n"
  end

  def test_sample_and_fx_opts
    refute_flags "unknown-opt", "sample :bd_haus, rate: 0.5, cutoff: 90, beat_stretch: 2, amp_slide: 1\n"
    assert_flags "unknown-opt", "sample :bd_haus, rat: 2\n"
    refute_flags "unknown-opt", "with_fx :reverb, room: 0.8, mix: 0.5, reps: 4 do\n  play 60\nend\n"
    assert_flags "unknown-opt", "with_fx :echo, room: 0.8 do\n  play 60\nend\n"
  end

  def test_invalid_values
    assert_flags "invalid-opt-value", "play 60, release: -1\n"
    assert_flags "invalid-opt-value", "use_synth :tb303\nplay 60, cutoff: 200\n"
    refute_flags "invalid-opt-value", "play 60, release: 2, amp: 0.5\n"
  end

  def test_live_loops
    assert_flags "live-loop-no-sleep", "live_loop :a do\n  play 60\nend\n"
    refute_flags "live-loop-no-sleep", "live_loop :a do\n  play 60\n  sleep 1\nend\n"
    refute_flags "live-loop-no-sleep", "live_loop :a do\n  sync :b\nend\n"
    refute_flags "live-loop-no-sleep", "define :beat do\n  sleep 1\nend\nlive_loop :a do\n  beat\nend\n"
    refute_flags "live-loop-no-sleep", "live_loop :a do\n  my_helper_from_elsewhere\nend\n"
    assert_flags "live-loop-no-sleep", "live_loop :a do\n  in_thread do\n    sleep 1\n  end\nend\n"
    assert_flags "live-loop-name", "live_loop do\n  sleep 1\nend\n"
    assert_flags "duplicate-live-loop", "live_loop :a do\n  sleep 1\nend\nlive_loop :a do\n  sleep 2\nend\n"
    assert_flags "fx-in-live-loop", "live_loop :a do\n  with_fx :reverb do\n    sleep 1\n  end\nend\n"
    refute_flags "fx-in-live-loop", "with_fx :reverb do\n  live_loop :a do\n    sleep 1\n  end\nend\n"
  end

  def test_loops
    assert_flags "loop-no-sleep", "loop do\n  play 60\nend\n"
    refute_flags "loop-no-sleep", "loop do\n  play 60\n  sleep 0.5\nend\n"
    assert_flags "unreachable-code", "loop do\n  sleep 1\nend\nplay 60\n"
    refute_flags "unreachable-code", "in_thread do\n  loop do\n    sleep 1\n  end\nend\nplay 60\n"
  end

  def test_timing
    assert_flags "negative-sleep", "sleep -1\n"
    assert_flags "invalid-bpm", "use_bpm 0\n"
    refute_flags "invalid-bpm", "use_bpm 120\n"
  end

  def test_functions
    assert_flags "undefined-function", "wobble 3\n"
    refute_flags "undefined-function", "define :wobble do |x|\n  play x\nend\nwobble 3\n"
    refute_flags "undefined-function", "x = 3\nplay x\nputs rrand(1, 2)\n"
    assert_flags "shadowed-builtin", "define :play do\nend\n"
    assert_flags "desktop-only", "load_example :haunted\n"
    assert_flags "deprecated", "use_sample_pack :foo\n"
    assert_flags "unknown-fn-opt", "live_loop :a, sycn: :b do\n  sleep 1\nend\n"
    refute_flags "unknown-fn-opt", "set_mixer_control! lpf: 30, lpf_slide: 16\n"
  end

  def test_syntax_errors_stop_ast_rules
    d = lint("live_loop :a do\n  play 60\n")
    assert_equal ["syntax"], d.map(&:rule).uniq - %w[final-newline]
    assert_match(/missing its `end`/, d.first.message)
  end

  def test_directives
    refute_flags "unknown-synth", "use_synth :nope # splint:disable-line unknown-synth\n"
    refute_flags "unknown-synth", "# splint:disable-next-line\nuse_synth :nope\n"
    refute_flags "unknown-synth", "# splint:disable unknown-synth\nuse_synth :nope\nuse_synth :nope2\n"
    assert_flags "unknown-synth", "# splint:disable unknown-synth\nuse_synth :a\n# splint:enable unknown-synth\nuse_synth :b\n"
  end

  def test_style_and_fix
    code = "live_loop :a do\nplay 60   \n\tsleep 1\nend"
    rules = rules_of(code)
    %w[trailing-whitespace no-tabs final-newline indentation].each { |r| assert_includes rules, r }
    cfg = Splint::Config.new({ "use_init" => false }, path: nil)
    fixed = Splint::Linter.new(k).fix("t.rb", code, cfg)
    assert_equal "live_loop :a do\n  play 60\n  sleep 1\nend\n", fixed
  end

  def test_beautify_matches_desktop_auto_align
    src = "live_loop :a do\nwith_fx :reverb do\nplay 60\n\nsleep 1\nend\nend\n"
    want = "live_loop :a do\n  with_fx :reverb do\n    play 60\n\n    sleep 1\n  end\nend\n"
    assert_equal want, Splint::Format.beautify(k, src)
  end

  def test_presets_and_config
    assert_flags "line-length", "play 60 #{'#' * 200}\n", preset: "all"
    refute_flags "line-length", "play 60 #{'#' * 200}\n"
    assert_empty lint("use_synth :nope\n", preset: "none")
    assert_flags "line-length", "play 60 #{'#' * 50}\n", rules: { "line-length" => { "max" => 20 } }
    d = lint("use_synth :nope\n", rules: { "unknown-synth" => "warning" }).find { |x| x.rule == "unknown-synth" }
    assert_equal :warning, d.severity
    assert_raises(Splint::ConfigError) { Splint::Config.new({ "rules" => { "nope" => "error" } }, path: nil) }
    assert_raises(Splint::ConfigError) { Splint::Config.new({ "rules" => { "line-length" => { "maxx" => 3 } } }, path: nil) }
    assert_raises(Splint::ConfigError) { Splint::Config.new({ "bogus" => 1 }, path: nil) }
  end

  def test_overrides_and_template
    Dir.mktmpdir do |d|
      File.write(File.join(d, ".splintrc"), "rules:\n  undefined-function: error\noverrides:\n  - files: [\"sketch/**\"]\n    rules:\n      undefined-function: off\n")
      cfg = Splint::Config.load(File.join(d, ".splintrc"))
      assert_equal :error, cfg.for_file(File.join(d, "a.rb")).rule("undefined-function")[0]
      assert_equal :off, cfg.for_file(File.join(d, "sketch/b.rb")).rule("undefined-function")[0]
      assert_equal File.join(d, ".splintrc"), Splint::Config.find(File.join(d, "deep", "er"))
    end
    t = Splint::Config.new(YAML.safe_load(Splint::Config.template), path: nil)
    assert_equal Splint::Rules.all.keys.sort, t.rules.keys.sort
  end

  def test_sonic_pi_examples_have_no_errors
    files = Dir[File.join(ROOT, "etc", "examples", "**", "*.rb")]
    skip "no examples in #{ROOT}" if files.empty?
    cfg = Splint::Config.new({ "use_init" => false }, path: nil)
    linter = Splint::Linter.new(k)
    errors = files.flat_map { |f| linter.lint(f, File.read(f, encoding: "UTF-8"), cfg) }.select { |d| d.severity == :error }
    assert_empty errors, errors.map { |d| "#{d.file}:#{d.line}: #{d.message}" }.join("\n")
  end

  def test_reports_render
    d = lint("use_synth :nope\n")
    %w[pretty compact json github sarif checkstyle].each do |f|
      out = Splint::Report.render(f, d, { "t.rb" => "use_synth :nope\n" })
      assert_match(/nope/, out, f)
    end
    JSON.parse(Splint::Report.render("sarif", d, {}))
  end
end
