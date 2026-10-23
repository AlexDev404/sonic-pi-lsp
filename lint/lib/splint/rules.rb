# frozen_string_literal: true

require "set"

module Splint
  # Base class for rules. A rule reports problems through #report; the
  # linter calls #on_call for every method call (in source order, with the
  # context it runs in), #on_source once for the text, and #on_finish at the
  # end of each file.
  class Rule
    class << self
      attr_reader :id, :severity, :category, :summary, :details, :options

      def meta(id, severity:, category:, summary:, details: "", options: {}, fixable: false)
        @id, @severity, @category, @summary, @details, @options, @fixable =
          id, severity, category, summary, details, options, fixable
        Rules.all[id] = self
      end

      def fixable? = @fixable
    end

    attr_reader :file, :options

    def initialize(file, options)
      @file = file
      @options = self.class.options.merge(options || {})
    end

    def id = self.class.id
    def k = file.knowledge

    def report(line, col, message, suggestion: nil)
      file.report(id, line, col, message, suggestion: suggestion)
    end

    def on_source; end
    def on_call(_call, _ctx); end
    def on_finish; end

    # A fixable rule rewrites the source.
    def fix(source) = source

    def fmt_sym(v) = v.is_a?(Symbol) ? ":#{v}" : v.inspect

    # Where argument i of a call starts, falling back to the call itself.
    def at(c, i)
      l, col = AST.pos(c.args[i])
      l ? [l, col + 1] : [c.line, c.col]
    end

    def did_you_mean(name, candidates)
      s = k.suggest(name, candidates)
      s.empty? ? "" : " (did you mean #{s.map { |x| ":#{x}" }.join(' or ')}?)"
    end
  end

  module Rules
    def self.all = (@all ||= {})

    TRIGGER_FNS = %i[play play_chord play_pattern play_pattern_timed].freeze
    SYNTH_DEFAULT_FNS = %i[use_synth_defaults use_merged_synth_defaults with_synth_defaults with_merged_synth_defaults].freeze
    SAMPLE_DEFAULT_FNS = %i[use_sample_defaults use_merged_sample_defaults with_sample_defaults with_merged_sample_defaults].freeze
    # Functions whose opts are free-form data, not settings.
    FREEFORM_OPTS = %i[cue set osc osc_send use_osc control live_audio sound_in sound_in_stereo
                       synth sample with_fx play play_chord play_pattern play_pattern_timed].freeze
    THREAD_FNS = %i[in_thread live_loop at].freeze
    DEFINE_FNS = %i[define defonce ndefine].freeze
    AUDIO_EXT = %w[.wav .wave .aif .aiff .flac .ogg .oga].freeze

    # ---- correctness ------------------------------------------------------

    class Syntax < Rule
      meta "syntax", severity: :error, category: :correctness,
           summary: "Ruby syntax errors (the code would not run at all)",
           details: "Sonic Pi code is Ruby. A syntax error stops the whole buffer from running; the most common cause is a `do` with no matching `end`."
      # Reported by the linter itself before the other rules run.
    end

    class UnknownSynth < Rule
      meta "unknown-synth", severity: :error, category: :correctness,
           summary: "use_synth, with_synth and synth with a synth Sonic Pi does not have",
           details: "Checked against the synths of the bundled Sonic Pi. An FX name gets a hint to use with_fx."

      def on_call(c, _ctx)
        return unless c.fcall? && Knowledge::SYNTH_NAME_FNS.include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.name?
        return if k.synth?(l.value)

        if k.fx?(l.value)
          report(*at(c, 0), ":#{l.value} is an FX, not a synth: use with_fx :#{l.value} do ... end")
        else
          report(*at(c, 0), "unknown synth :#{l.value}#{did_you_mean(l.value, k.synths)}")
        end
      end
    end

    class UnknownFX < Rule
      meta "unknown-fx", severity: :error, category: :correctness,
           summary: "with_fx with an FX Sonic Pi does not have"

      def on_call(c, _ctx)
        return unless c.fcall? && %i[with_fx use_fx].include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.name?
        return if k.fx?(l.value) || l.value.to_s == "none"

        if k.synth?(l.value)
          report(*at(c, 0), ":#{l.value} is a synth, not an FX: use_synth :#{l.value}")
        else
          report(*at(c, 0), "unknown FX :#{l.value}#{did_you_mean(l.value, k.fx)}")
        end
      end
    end

    class UnknownSample < Rule
      meta "unknown-sample", severity: :error, category: :correctness,
           summary: "built-in samples (sample :name) that do not exist"

      def on_call(c, _ctx)
        return unless c.fcall? && Knowledge::SAMPLE_NAME_FNS.include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.sym?
        return if k.sample?(l.value)

        report(*at(c, 0), "unknown sample :#{l.value}#{did_you_mean(l.value, k.samples)}")
      end
    end

    class UnknownOpt < Rule
      meta "unknown-opt", severity: :warning, category: :correctness,
           summary: "opts a synth, FX or sample does not take, which Sonic Pi silently ignores (cuttoff:, amp_sldie: ...)",
           details: "play and friends are checked against the current synth (as set by use_synth or with_synth), with_fx against the FX, sample against the sample player. Inside define, where the synth is not known, opts are checked against every synth. Sonic Pi does not complain about these: the opt just does nothing, which is usually a typo or an opt meant for another synth.",
           options: { "allow" => [] }

      def on_call(c, ctx)
        return unless c.fcall?
        return if c.opts.empty?

        allowed, what = allowed_for(c, ctx)
        return unless allowed

        extra = Set.new(Array(options["allow"]).map(&:to_sym))
        c.opts.each do |o|
          key = o.key.to_sym
          next if allowed.include?(key) || extra.include?(key) || k.slide_variant?(key, allowed)

          report(o.line, o.col, "#{key}: is not an opt of #{what}, so it does nothing#{did_you_mean(key, allowed)}")
        end
      end

      def all_synth_opts
        @all_synth_opts ||= k.synths.reduce(k.trigger_opts.dup) { |s, n| s | (k.play_opts(n) || []) }
      end

      def allowed_for(c, ctx)
        name = c.sym
        if name == :synth
          l = c.arg_lit(0)
          return nil unless l&.name? && k.synth?(l.value)

          [k.play_opts(l.value.to_sym), "synth :#{l.value}"]
        elsif TRIGGER_FNS.include?(name) || SYNTH_DEFAULT_FNS.include?(name)
          base = Set.new(k.doc_opts(name))
          if ctx.synth && k.synth?(ctx.synth)
            [k.play_opts(ctx.synth) | base, "synth :#{ctx.synth}"]
          else
            [all_synth_opts | base, "any synth"]
          end
        elsif Knowledge::SAMPLE_NAME_FNS.include?(name) && name != :load_samples || SAMPLE_DEFAULT_FNS.include?(name)
          [k.sample_opts | Set.new(k.doc_opts(name)), "sample"]
        elsif name == :with_fx
          l = c.arg_lit(0)
          return nil unless l&.name? && k.fx?(l.value)

          [k.fx_opts(l.value.to_sym), "FX :#{l.value}"]
        end
      end
    end

    class UnknownFnOpt < Rule
      meta "unknown-fn-opt", severity: :warning, category: :correctness,
           summary: "opts that a Sonic Pi function does not document (live_loop :x, sycn: ...)"

      def on_call(c, _ctx)
        return unless c.fcall? && !c.opts.empty?
        return if FREEFORM_OPTS.include?(c.sym) || UnknownOpt.allowed_fn?(c.sym)

        doc = k.docs[c.sym]
        return unless doc && doc[:opts].is_a?(Hash) && !doc[:opts].empty?

        allowed = doc[:opts].keys
        c.opts.each do |o|
          next if allowed.include?(o.key.to_sym) || k.slide_variant?(o.key.to_sym, allowed)

          report(o.line, o.col, "#{o.key}: is not an opt of #{c.name}, so it does nothing#{did_you_mean(o.key, allowed)}")
        end
      end
    end

    class UnknownOpt
      def self.allowed_fn?(name)
        TRIGGER_FNS.include?(name) || SYNTH_DEFAULT_FNS.include?(name) || SAMPLE_DEFAULT_FNS.include?(name) ||
          Knowledge::SAMPLE_NAME_FNS.include?(name)
      end
    end

    class InvalidOptValue < Rule
      meta "invalid-opt-value", severity: :error, category: :correctness,
           summary: "literal opt values a synth, FX or sample rejects (release: -1, cutoff: 200)",
           details: "Uses the validations in Sonic Pi's own synth definitions, the same checks the server makes when use_arg_checks is on."

      def on_call(c, ctx)
        return unless c.fcall? && !c.opts.empty?

        info, what = target(c, ctx)
        return unless info

        lits = {}
        c.opts.each do |o|
          l = AST.literal(o.value)
          lits[o.key.to_sym] = l.value if l&.num?
        end
        return if lits.empty?

        k.invalid_values(info, lits).each do |key, msg|
          o = c.opt(key)
          report(o.line, o.col, "#{key}: #{lits[key]} is invalid for #{what}: it #{msg}")
        end
      end

      def target(c, ctx)
        name = c.sym
        if name == :synth
          l = c.arg_lit(0)
          l&.name? && k.synth?(l.value) ? [k.synth_info(l.value), "synth :#{l.value}"] : nil
        elsif TRIGGER_FNS.include?(name) || SYNTH_DEFAULT_FNS.include?(name)
          ctx.synth && k.synth?(ctx.synth) ? [k.synth_info(ctx.synth), "synth :#{ctx.synth}"] : nil
        elsif %i[sample].include?(name) || SAMPLE_DEFAULT_FNS.include?(name)
          [k.synth_info(:stereo_player), "sample"]
        elsif name == :with_fx
          l = c.arg_lit(0)
          l&.name? && k.fx?(l.value) ? [k.fx_info(l.value), "FX :#{l.value}"] : nil
        end
      end
    end

    class UnknownNote < Rule
      meta "unknown-note", severity: :error, category: :correctness,
           summary: "note names Sonic Pi cannot read (play :h4, note: :cs)"

      NOTE_FNS = %i[play play_chord play_pattern play_pattern_timed note note_info midi_note_on midi_note_off midi].freeze
      TONIC_FNS = %i[chord scale chord_degree degree chord_invert].freeze

      def on_call(c, _ctx)
        return unless c.fcall?

        i = if NOTE_FNS.include?(c.sym) || c.sym == :chord || c.sym == :scale then 0
            elsif c.sym == :chord_degree || c.sym == :degree then 1
            end
        check(c.arg_lit(i), *at(c, i)) if i
        if (o = c.opt(:note)) && (c.sym == :synth || NOTE_FNS.include?(c.sym) || c.sym == :control)
          check(AST.literal(o.value), o.line, o.col)
        end
      end

      def check(lit, line, col)
        return unless lit

        values = lit.kind == :array ? lit.value : [lit]
        values.each do |v|
          next unless v&.name?
          next if %w[r rest].include?(v.value.to_s)
          next if k.note?(v.value)

          report(line, col, "#{fmt_sym(v.value)} is not a note (notes look like :c4, :Eb3, :fs2 or :r for a rest)")
        end
      end
    end

    class UnknownChord < Rule
      meta "unknown-chord", severity: :error, category: :correctness,
           summary: "chord names Sonic Pi does not have (chord :e, :minr)"

      def on_call(c, _ctx)
        return unless c.fcall?

        l = case c.sym
            when :chord, :chord_invert then c.arg_lit(1)
            end
        return unless l&.name?
        return if k.chord?(l.value)

        report(*at(c, 1), "unknown chord #{fmt_sym(l.value)}#{did_you_mean(l.value, k.chords.select { |x| x =~ /\A[a-z]/ })}")
      end
    end

    class UnknownScale < Rule
      meta "unknown-scale", severity: :error, category: :correctness,
           summary: "scale names Sonic Pi does not have (scale :c, :majr)"

      def on_call(c, _ctx)
        return unless c.fcall?

        l = case c.sym
            when :scale then c.arg_lit(1)
            when :chord_degree, :degree then c.arg_lit(2)
            end
        return unless l&.name?
        return if k.scale?(l.value)

        report(*at(c, c.sym == :scale ? 1 : 2), "unknown scale #{fmt_sym(l.value)}#{did_you_mean(l.value, k.scales)}")
      end
    end

    class LiveLoopNoSleep < Rule
      meta "live-loop-no-sleep", severity: :error, category: :correctness,
           summary: "live_loops that never sleep or sync (Sonic Pi stops them with an error)",
           details: "A live_loop body must advance time with sleep, sync or a function that does (play_pattern_timed, or your own define that sleeps). Calls to functions the linter cannot see into count as sleeping."

      def on_call(c, _ctx)
        return unless c.fcall? && c.sym == :live_loop && c.block
        return unless file.advances_time(c.block.body) == :no

        name = c.arg_lit(0)
        label = name&.name? ? "live_loop :#{name.value}" : "this live_loop"
        report(c.line, c.col, "#{label} never sleeps or syncs: add a sleep (e.g. sleep 1) or a sync")
      end
    end

    class LoopNoSleep < Rule
      meta "loop-no-sleep", severity: :error, category: :correctness,
           summary: "loop do ... end and while true loops that never sleep (they lock up the run)"

      def on_call(c, _ctx)
        return unless c.fcall? && c.sym == :loop && c.block
        return unless file.advances_time(c.block.body) == :no
        return if file.breaks?(c.block.body)

        report(c.line, c.col, "this loop never sleeps or syncs, so it spins forever: add a sleep")
      end
    end

    class LiveLoopName < Rule
      meta "live-loop-name", severity: :error, category: :correctness,
           summary: "live_loops without a name"

      def on_call(c, _ctx)
        return unless c.fcall? && c.sym == :live_loop
        return unless c.args.empty?

        report(c.line, c.col, "live_loop needs a name: live_loop :drums do ... end")
      end
    end

    class NegativeSleep < Rule
      meta "negative-sleep", severity: :error, category: :correctness,
           summary: "sleep or wait with a negative time"

      def on_call(c, _ctx)
        return unless c.fcall? && %i[sleep wait].include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.num? && l.value.negative?

        report(*at(c, 0), "#{c.name} #{l.value}: time cannot go backwards (use time_warp to shift timing)")
      end
    end

    class InvalidBpm < Rule
      meta "invalid-bpm", severity: :error, category: :correctness,
           summary: "use_bpm or with_bpm with a tempo of zero or less"

      def on_call(c, _ctx)
        return unless c.fcall? && %i[use_bpm with_bpm].include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.num? && l.value <= 0

        report(*at(c, 0), "#{c.name} #{l.value}: the tempo must be greater than 0")
      end
    end

    # ---- suspicious -------------------------------------------------------

    class UndefinedFunction < Rule
      meta "undefined-function", severity: :warning, category: :suspicious,
           summary: "calls to functions that are not Sonic Pi's, Ruby's, or defined in the file",
           details: "Functions defined in ~/.sonic-pi/config/init.rb count as defined when use_init is on. List others (from run_file, say) under globals in .splintrc."

      def on_call(c, _ctx)
        return unless c.fcall? && c.name
        return if c.name.match?(/\A[A-Z]/)
        return if file.known_function?(c.name)

        cands = k.docs.keys + file.defined_names.to_a.map(&:to_sym)
        report(c.line, c.col, "undefined function or variable `#{c.name}`#{did_you_mean(c.name, cands).sub(':', '').gsub(' or :', ' or ')}")
      end
    end

    class DuplicateLiveLoop < Rule
      meta "duplicate-live-loop", severity: :warning, category: :suspicious,
           summary: "two live_loops with the same name (only the last one plays)"

      def on_source = (@seen = {})

      def on_call(c, _ctx)
        return unless c.fcall? && c.sym == :live_loop

        l = c.arg_lit(0)
        return unless l&.name?

        n = l.value.to_s
        if (first = @seen[n])
          report(c.line, c.col, "live_loop :#{n} is already defined on line #{first}; this one replaces it, so the first never plays")
        else
          @seen[n] = c.line
        end
      end
    end

    class UnreachableCode < Rule
      meta "unreachable-code", severity: :warning, category: :suspicious,
           summary: "code after an endless loop do ... end, which never runs",
           details: "loop do ... end blocks the thread it runs in. Use live_loop or in_thread to run code beside it."

      def on_source
        file.statement_lists.each do |stmts|
          stmts.each_with_index do |st, i|
            c = AST.call(st)
            next unless c&.fcall? && c.sym == :loop && c.block && !file.breaks?(c.block.body)

            rest = stmts[(i + 1)..].reject { |x| x.is_a?(Array) && x[0] == :void_stmt }
            next if rest.empty?

            l, col = AST.pos(rest.first) || [c.line, 0]
            report(l, col + 1, "this never runs: the loop on line #{c.line} never ends (use live_loop or in_thread for loops that run side by side)")
          end
        end
      end
    end

    class ShadowedBuiltin < Rule
      meta "shadowed-builtin", severity: :warning, category: :suspicious,
           summary: "define or def that replaces one of Sonic Pi's own functions"

      def on_source
        file.definitions.each do |name, (line, col)|
          next unless k.documented?(name)

          report(line, col, "`#{name}` replaces Sonic Pi's own #{name}; pick another name")
        end
      end
    end

    class SyncWithoutCue < Rule
      meta "sync-without-cue", severity: :info, category: :suspicious,
           summary: "sync on a name that nothing in the file cues",
           details: "Fine when the cue comes from another buffer, OSC or MIDI; a typo otherwise."

      def on_call(c, _ctx)
        return unless c.fcall? && %i[sync sync_bpm].include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.name?

        n = l.value.to_s
        return if n.start_with?("/osc", "/midi", "/link", "/cue", "/live_loop", "/set") && file.cue_names.include?(n.split("/").last.to_s)
        return if n.start_with?("/osc", "/midi", "/link") || n.include?("*")
        return if file.cue_names.include?(n.delete_prefix("/cue/").delete_prefix("/live_loop/").delete_prefix("/set/"))

        report(*at(c, 0), "nothing in this file cues #{fmt_sym(l.value)} (no live_loop, cue or set of that name)")
      end
    end

    class UnusedDefine < Rule
      meta "unused-define", severity: :info, category: :suspicious,
           summary: "functions defined with define that are never called"

      def on_source
        file.definitions.each do |name, (line, col)|
          next if file.called_names.include?(name) || file.symbol_names.count(name) > 1

          report(line, col, "`#{name}` is defined but never called")
        end
      end
    end

    class SamplePathMissing < Rule
      meta "sample-path-missing", severity: :warning, category: :suspicious,
           summary: "sample \"path\" where the file or folder does not exist (or is an mp3)",
           options: { "search" => [] }

      def on_call(c, _ctx)
        return unless c.fcall? && Knowledge::SAMPLE_NAME_FNS.include?(c.sym)

        c.args.each_with_index do |a, i|
          l = AST.literal(a)
          next unless l&.str?

          s = l.value
          ext = File.extname(s).downcase
          next unless s.include?("/") || s.include?("\\") || AUDIO_EXT.include?(ext) || ext == ".mp3"

          if ext == ".mp3"
            report(*at(c, i), "Sonic Pi cannot play mp3 files; convert it to wav, aiff, flac or ogg")
            next
          end
          next if file.path_exists?(s, options["search"])

          report(*at(c, i), "no such sample file or folder: #{s}")
        end
      end
    end

    class RunFileMissing < Rule
      meta "run-file-missing", severity: :warning, category: :suspicious,
           summary: "run_file with a path that does not exist"

      def on_call(c, _ctx)
        return unless c.fcall? && %i[run_file].include?(c.sym)

        l = c.arg_lit(0)
        return unless l&.str?
        return if file.path_exists?(l.value, [])

        report(*at(c, 0), "no such file: #{l.value}")
      end
    end

    class AmpTooHigh < Rule
      meta "amp-too-high", severity: :info, category: :suspicious,
           summary: "very loud literal amp: values, which distort",
           options: { "max" => 5 }

      def on_call(c, _ctx)
        return unless c.fcall?

        o = c.opt(:amp)
        return unless o

        l = AST.literal(o.value)
        return unless l&.num? && l.value > options["max"].to_f

        report(o.line, o.col, "amp: #{l.value} is very loud and will distort (over #{options['max']}); use the volume or mixer instead")
      end
    end

    # ---- performance ------------------------------------------------------

    class FxInLiveLoop < Rule
      meta "fx-in-live-loop", severity: :info, category: :performance,
           summary: "with_fx directly inside a live_loop, which builds a new FX every time round",
           details: "Each with_fx starts a new FX synth. Inside a live_loop that happens on every iteration, which costs CPU and can glitch. Wrap the live_loop in the with_fx instead."

      def on_call(c, ctx)
        return unless c.fcall? && c.sym == :with_fx && ctx.parent == :live_loop

        report(c.line, c.col, "with_fx inside live_loop :#{ctx.loop_name} starts a new FX on every iteration; put the live_loop inside the with_fx")
      end
    end

    class NestedLiveLoop < Rule
      meta "nested-live-loop", severity: :warning, category: :performance,
           summary: "live_loop defined inside another loop (redefined on every iteration)"

      def on_call(c, ctx)
        return unless c.fcall? && c.sym == :live_loop && ctx.in_loop?

        report(c.line, c.col, "this live_loop is defined inside a loop, so it is redefined on every iteration; move it to the top level")
      end
    end

    class ThreadInLoop < Rule
      meta "thread-in-loop", severity: :info, category: :performance,
           summary: "in_thread inside a loop without a name: a new thread every iteration"

      def on_call(c, ctx)
        return unless c.fcall? && c.sym == :in_thread && ctx.in_loop? && !c.opt(:name)

        report(c.line, c.col, "in_thread inside a loop starts a new thread on every iteration; give it name: so only one runs")
      end
    end

    # ---- compatibility ----------------------------------------------------

    class DesktopOnly < Rule
      meta "desktop-only", severity: :warning, category: :compat,
           summary: "functions that need the desktop editor (load_buffer, load_example)"

      def on_call(c, _ctx)
        return unless c.fcall? && (why = Knowledge::DESKTOP_ONLY[c.sym])

        report(c.line, c.col, "#{c.name} #{why}")
      end
    end

    class Deprecated < Rule
      meta "deprecated", severity: :warning, category: :compat,
           summary: "functions Sonic Pi no longer documents (sample packs)"

      def on_call(c, _ctx)
        return unless c.fcall? && (why = Knowledge::DEPRECATED[c.sym])

        report(c.line, c.col, "#{c.name} is deprecated: #{why}")
      end
    end

    # ---- style ------------------------------------------------------------

    class LineLength < Rule
      meta "line-length", severity: :off, category: :style,
           summary: "lines longer than max", options: { "max" => 120, "ignore_comments" => false }

      def on_source
        max = options["max"].to_i
        file.lines.each_with_index do |l, i|
          next if l.length <= max
          next if options["ignore_comments"] && l.lstrip.start_with?("#")

          report(i + 1, max + 1, "line is #{l.length} characters long (max #{max})")
        end
      end
    end

    class TrailingWhitespace < Rule
      meta "trailing-whitespace", severity: :info, category: :style, fixable: true,
           summary: "spaces or tabs at the end of a line"

      def on_source
        file.lines.each_with_index do |l, i|
          next unless l =~ /[ \t]+\z/

          report(i + 1, $~.begin(0) + 1, "trailing whitespace")
        end
      end

      def fix(source) = source.gsub(/[ \t]+$/, "")
    end

    class NoTabs < Rule
      meta "no-tabs", severity: :info, category: :style, fixable: true,
           summary: "tab characters used for indentation", options: { "width" => 2 }

      def on_source
        file.lines.each_with_index do |l, i|
          next unless (m = l.match(/\A[ ]*\t/))

          report(i + 1, m.end(0), "tab used for indentation (Sonic Pi indents with #{options['width']} spaces)")
        end
      end

      def fix(source)
        source.gsub(/^[ \t]+/) { |ws| ws.gsub("\t", " " * options["width"].to_i) }
      end
    end

    class FinalNewline < Rule
      meta "final-newline", severity: :info, category: :style, fixable: true,
           summary: "files that do not end with a newline"

      def on_source
        src = file.source
        return if src.empty? || src.end_with?("\n")

        report(file.lines.size, file.lines.last.to_s.length + 1, "no newline at the end of the file")
      end

      def fix(source) = source.empty? || source.end_with?("\n") ? source : "#{source}\n"
    end

    class Indentation < Rule
      meta "indentation", severity: :info, category: :style, fixable: true,
           summary: "indentation that differs from Sonic Pi's auto-align (what the desktop does on Run)",
           details: "Fix it with `sonicpi fmt -w FILE` or `sonicpi lint --fix`."

      def on_source
        return if file.syntax_error?

        pretty = Format.beautify(file.knowledge, file.source).split("\n", -1)
        file.lines.each_with_index do |l, i|
          want = pretty[i]
          next if want.nil? || l.strip.empty?

          have_ws = l[/\A[ \t]*/]
          want_ws = want[/\A */]
          next if have_ws == want_ws

          have = if have_ws.include?("\t") then "tabs"
                 elsif have_ws.empty? then "none"
                 else "#{have_ws.length}"
                 end
          report(i + 1, 1, "indent this line with #{want_ws.length} spaces, not #{have} (as Sonic Pi's auto-align does)")
        end
      end

      def fix(source)
        Format.beautify(file.knowledge, source)
      rescue StandardError
        source
      end
    end

    class MaxNesting < Rule
      meta "max-nesting", severity: :off, category: :style,
           summary: "blocks nested more than max deep", options: { "max" => 5 }

      def on_call(c, ctx)
        return unless c.block && ctx.depth + 1 > options["max"].to_i

        report(c.line, c.col, "blocks nested #{ctx.depth + 1} deep (max #{options['max']}); pull some out into a define")
      end
    end

    class MaxLines < Rule
      meta "max-lines", severity: :off, category: :style,
           summary: "files longer than max lines", options: { "max" => 500 }

      def on_source
        n = file.lines.size
        report(n, 1, "file has #{n} lines (max #{options['max']})") if n > options["max"].to_i
      end
    end

    class CommentSpace < Rule
      meta "comment-space", severity: :off, category: :style, fixable: true,
           summary: "comments without a space after #"

      def on_source
        file.comments.each do |line, col, text|
          next if text.match?(/\A#([ !#]|$|splint:)/) || text.start_with?("#__nosave__")

          report(line, col, "put a space after # in comments")
        end
      end

      def fix(source)
        lines = source.split("\n", -1)
        file.comments.each do |line, col, text|
          next if text.match?(/\A#([ !#]|$|splint:)/)

          l = lines[line - 1]
          lines[line - 1] = "#{l[0...(col - 1)]}# #{text[1..]}" if l && l[col - 1] == "#"
        end
        lines.join("\n")
      end
    end
  end
end
