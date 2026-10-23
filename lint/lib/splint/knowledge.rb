# frozen_string_literal: true

require "set"

module Splint
  # What Sonic Pi itself knows: its synths, FX, samples, notes, chords,
  # scales and language. It is read from the bundled Sonic Pi server, so the
  # linter always matches the version it ships with.
  class Knowledge
    # Functions that advance time, so a loop that calls one does not spin.
    TIME_ADVANCING = %i[sleep wait sync sync_bpm play_pattern_timed play_pattern].freeze

    # Functions that only make sense in the desktop app: they replace the
    # contents of an editor buffer.
    DESKTOP_ONLY = {
      load_buffer: "replaces the desktop editor's current buffer, which the CLI does not have; use `sonicpi buffer load N FILE`",
      load_example: "loads an example into the desktop editor, which the CLI does not have",
    }.freeze

    # Functions Sonic Pi still accepts but no longer documents.
    DEPRECATED = {
      use_sample_pack: "sample packs were replaced by sample paths: sample \"~/samples/\", :name",
      with_sample_pack: "sample packs were replaced by sample paths: sample \"~/samples/\", :name",
      use_sample_pack_as: "sample packs were replaced by sample paths: sample \"~/samples/\", :name",
      with_sample_pack_as: "sample packs were replaced by sample paths: sample \"~/samples/\", :name",
    }.freeze

    # Calls that take a synth name as their first argument.
    SYNTH_NAME_FNS = %i[use_synth with_synth synth].freeze
    # Calls that take a sample name as their first argument.
    SAMPLE_NAME_FNS = %i[sample sample_duration sample_info load_sample sample_buffer sample_loaded? sample_free load_samples].freeze

    attr_reader :root, :version, :synths, :fx, :samples, :docs, :chords, :scales

    def self.load(root)
      @cache ||= {}
      @cache[root] ||= new(root)
    end

    def initialize(root)
      @root = root
      server = File.join(root, "app", "server", "ruby")
      raise ArgumentError, "no Sonic Pi server in #{root}" unless File.exist?(File.join(server, "core.rb"))

      verbose, $VERBOSE = $VERBOSE, nil
      begin
        require File.join(server, "core.rb")
        require File.join(server, "lib/sonicpi/synths/synthinfo")
        %w[lang/core lang/sound lang/midi lang/western_theory lang/maths].each do |m|
          require File.join(server, "lib/sonicpi", m)
        end
      ensure
        $VERBOSE = verbose
      end

      @info = SonicPi::Synths::SynthInfo
      @synths = Set.new(@info.all_synths)
      @fx = Set.new(@info.all_fx)
      @samples = Set.new(@info.all_samples)
      @docs = SonicPi::Lang::Core.class_variable_get(:@@docs)
      @chords = Set.new(SonicPi::Chord::CHORD.keys.map(&:to_s))
      @scales = Set.new(SonicPi::Scale::SCALE.keys.map(&:to_s))
      @lang_methods = Set.new
      [SonicPi::Lang::Core, SonicPi::Lang::Sound, SonicPi::Lang::Midi,
       SonicPi::Lang::WesternTheory, SonicPi::Lang::Maths].each do |m|
        (m.instance_methods(false) + m.private_instance_methods(false)).each { |n| @lang_methods << n }
      end
      @ruby_methods = Set.new(Kernel.instance_methods + Kernel.private_instance_methods +
                              Object.instance_methods + Object.private_instance_methods)
      @version = read_version(server)
      @opt_cache = {}
    end

    def read_version(server)
      # Sonic Pi 5 keeps its version in a VERSION file; 4.x in runtime.rb.
      file = File.join(@root, "VERSION")
      return File.read(file).strip if File.file?(file)

      src = File.read(File.join(server, "lib/sonicpi/runtime.rb"))
      m = src.match(/^\s*@version = Version\.new\(([^)]*)\)/)
      m ? m[1].split(",").map { |s| s.strip.delete('"\'') }.join(".") : "unknown"
    rescue StandardError
      "unknown"
    end

    def synth?(name) = @synths.include?(name.to_sym)
    def fx?(name) = @fx.include?(name.to_sym)
    def sample?(name) = @samples.include?(name.to_sym)
    def documented?(name) = @docs.key?(name.to_sym)
    def api?(name) = documented?(name) || (@lang_methods.include?(name.to_sym) && !name.to_s.start_with?("__"))
    def ruby?(name) = @ruby_methods.include?(name.to_sym)
    def chord?(name) = @chords.include?(name.to_s)
    def scale?(name) = @scales.include?(name.to_s)

    def synth_info(name) = @info.get_info(name.to_sym)
    def fx_info(name) = @info.get_info(:"fx_#{name}")

    def note?(value)
      SonicPi::Note.resolve_midi_note(value)
      true
    rescue StandardError
      false
    end

    def doc_opts(fn)
      d = @docs[fn.to_sym]
      d && d[:opts] ? d[:opts].keys : []
    end

    # Opts every synth trigger accepts besides the synth's own args.
    def trigger_opts
      @trigger_opts ||= Set.new(doc_opts(:play) + doc_opts(:synth) + %i[on slide pitch duration])
    end

    def play_opts(synth)
      @opt_cache[[:synth, synth]] ||= begin
        i = synth_info(synth)
        i ? trigger_opts | i.arg_defaults.keys | aliases(i).keys : nil
      end
    end

    def fx_opts(fx)
      @opt_cache[[:fx, fx]] ||= begin
        i = fx_info(fx)
        i ? Set.new(doc_opts(:with_fx)) | i.arg_defaults.keys | aliases(i).keys : nil
      end
    end

    def sample_opts
      @opt_cache[:sample] ||= begin
        opts = Set.new(doc_opts(:sample) + %i[on slide path])
        %i[stereo_player basic_stereo_player mono_player basic_mono_player].each do |p|
          i = @info.get_info(p)
          opts |= i.arg_defaults.keys | aliases(i).keys if i
        end
        opts
      end
    end

    # Opts a synth renames before triggering (sample players take cutoff:
    # for lpf:), read from the alias_opts! calls in its munge_opts.
    def aliases(info)
      m = info.method(:munge_opts)
      file, line = m.source_location
      return {} unless file && File.exist?(file)

      @source_lines ||= {}
      lines = (@source_lines[file] ||= File.readlines(file))
      indent = lines[line - 1][/\A */]
      body = lines[line..].take_while { |l| !l.match?(/\A#{indent}end\b/) }.join
      body.scan(/alias_opts!\(:(\w+),\s*:(\w+)/).to_h { |a, b| [a.to_sym, b.to_sym] }
    rescue StandardError
      {}
    end

    def slide_variant?(key, opts)
      m = key.to_s.match(/\A(.+)_slide(_shape|_curve)?\z/)
      m && opts.include?(m[1].to_sym)
    end

    # Checks literal opt values against the synth's own validations.
    # Returns [opt, message] pairs.
    def invalid_values(info, literals)
      return [] unless info

      defaults = info.arg_defaults.dup
      # Some defaults name another opt (decay_level: :sustain_level).
      defaults.each { |k, v| defaults[k] = defaults[v] if v.is_a?(Symbol) && defaults.key?(v) }
      args = defaults.merge(literals)
      out = []
      literals.each_key do |k|
        info.arg_validations(k).each do |fn, msg|
          ok = begin
            fn.call(args)
          rescue StandardError
            true
          end
          out << [k, msg] unless ok
        end
      end
      out
    end

    # Close matches for a misspelled name, nearest first.
    def suggest(name, candidates, max: 3)
      name = name.to_s
      scored = candidates.map(&:to_s).map { |c| [Splint.distance(name, c), c] }
      limit = [2, name.length / 3].max
      scored.select { |d, _| d <= limit }.sort.first(max).map(&:last)
    end
  end

  # Levenshtein distance.
  def self.distance(a, b)
    return b.length if a.empty?
    return a.length if b.empty?

    prev = (0..b.length).to_a
    a.each_char.with_index(1) do |ca, i|
      cur = [i]
      b.each_char.with_index(1) do |cb, j|
        cur << [prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ca == cb ? 0 : 1)].min
      end
      prev = cur
    end
    prev.last
  end
end
