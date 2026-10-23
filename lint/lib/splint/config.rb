# frozen_string_literal: true

require "yaml"
require "json"
require "set"

module Splint
  class ConfigError < StandardError; end

  RC_NAMES = %w[.splintrc .splintrc.yml .splintrc.yaml .splintrc.json].freeze
  PRESETS = %w[recommended strict all none].freeze
  FORMATS = %w[pretty compact json github sarif checkstyle].freeze
  FAIL_ON = %w[error warning info never].freeze

  TOP_KEYS = %w[extends fail_on max_warnings format include exclude globals use_init
                inline_config report_unused_disable rules overrides].freeze

  # The effective configuration: a .splintrc, with any command-line
  # overrides applied, resolved for one file.
  class Config
    attr_reader :path, :dir, :data, :fail_on, :max_warnings, :format, :include, :exclude,
                :inline, :report_unused_disable, :globals

    # Finds the nearest rc file at or above dir.
    def self.find(dir)
      dir = File.expand_path(dir)
      loop do
        RC_NAMES.each do |n|
          p = File.join(dir, n)
          return p if File.file?(p)
        end
        parent = File.dirname(dir)
        return nil if parent == dir

        dir = parent
      end
    end

    def self.read(path)
      text = File.read(path)
      data = if path.end_with?(".json") || text.lstrip.start_with?("{")
               JSON.parse(text)
             else
               YAML.safe_load(text, aliases: false) || {}
             end
      raise ConfigError, "#{path}: the top level must be a mapping" unless data.is_a?(Hash)

      data
    rescue Psych::SyntaxError, JSON::ParserError => e
      raise ConfigError, "#{path}: #{e.message}"
    end

    def self.load(path, cli = {})
      new(read(path), path: path, cli: cli)
    end

    def self.defaults(cli = {})
      new({ "extends" => "recommended" }, path: nil, cli: cli)
    end

    def initialize(data, path:, cli: {})
      @path = path
      @dir = path ? File.dirname(File.expand_path(path)) : Dir.pwd
      @data = data
      @cli = cli
      validate!(data, path || "defaults")
      preset = (cli[:preset] || data["extends"] || "recommended").to_s
      raise ConfigError, "#{where}: extends must be one of #{PRESETS.join(', ')}, got #{preset.inspect}" unless PRESETS.include?(preset)

      @base = Config.preset(preset)
      @rules = merge_rules(@base, data["rules"] || {}, where)
      cli_rules(@rules)
      @fail_on = (cli[:fail_on] || data["fail_on"] || "error").to_s
      raise ConfigError, "#{where}: fail_on must be one of #{FAIL_ON.join(', ')}" unless FAIL_ON.include?(@fail_on)

      @max_warnings = (cli[:max_warnings] || data["max_warnings"] || -1).to_i
      @format = (cli[:format] || data["format"] || "pretty").to_s
      raise ConfigError, "#{where}: format must be one of #{FORMATS.join(', ')}" unless FORMATS.include?(@format)

      @include = Array(data["include"] || ["**/*.rb"])
      @exclude = Array(data["exclude"] || [])
      @inline = cli.key?(:inline) ? cli[:inline] : data.fetch("inline_config", true) != false
      @report_unused_disable = cli[:report_unused_disable] || data["report_unused_disable"] == true
      @globals = Set.new(Array(data["globals"]).map(&:to_s))
      @use_init = data.fetch("use_init", true) != false
      @overrides = Array(data["overrides"])
    end

    def where = @path || "configuration"

    # Severities for a preset.
    def self.preset(name)
      Rules.all.each_with_object({}) do |(id, k), h|
        sev = k.severity
        sev = case name
              when "none" then :off
              when "all" then sev == :off ? :info : sev
              when "strict"
                s = sev == :off || sev == :info ? :warning : sev
                s = :error if s == :warning && %i[correctness suspicious].include?(k.category)
                s
              else sev
              end
        h[id] = [sev, {}]
      end
    end

    def validate!(data, where)
      unknown = data.keys - TOP_KEYS
      return if unknown.empty?

      raise ConfigError, "#{where}: unknown setting #{unknown.map(&:inspect).join(', ')} (known: #{TOP_KEYS.join(', ')})"
    end

    def parse_rule(id, value, where)
      klass = Rules.all[id]
      unless klass || id == "unused-disable"
        s = Splint.suggest_rule(id)
        raise ConfigError, "#{where}: unknown rule #{id.inspect}#{s}"
      end
      sev, opts = case value
                  when Hash
                    v = value.transform_keys(&:to_s)
                    [v.delete("severity") || v.delete("level"), v]
                  when Array then [value[0], (value[1] || {}).transform_keys(&:to_s)]
                  else [value, {}]
                  end
      sev = sev.nil? ? nil : Splint.severity(sev)
      if klass
        bad = opts.keys - klass.options.keys
        unless bad.empty?
          known = klass.options.empty? ? "it takes none" : "it takes #{klass.options.keys.join(', ')}"
          raise ConfigError, "#{where}: rule #{id} has no option #{bad.map(&:inspect).join(', ')} (#{known})"
        end
      end
      [sev, opts]
    rescue ConfigError => e
      raise ConfigError, e.message.start_with?(where) ? e.message : "#{where}: rule #{id}: #{e.message}"
    end

    def merge_rules(base, rules, where)
      raise ConfigError, "#{where}: rules must be a mapping of rule ids to settings" unless rules.is_a?(Hash)

      out = base.transform_values { |s, o| [s, o.dup] }
      rules.each do |id, v|
        id = id.to_s
        sev, opts = parse_rule(id, v, where)
        cur_sev, cur_opts = out[id] || [:off, {}]
        out[id] = [sev || (cur_sev == :off ? Config.on_severity(id) : cur_sev), cur_opts.merge(opts)]
      end
      out
    end

    # The severity a rule gets when it is turned on without one.
    def self.on_severity(id)
      d = Rules.all[id]&.severity || :warning
      d == :off ? :warning : d
    end

    def cli_rules(rules)
      (@cli[:rules] || []).each do |id, sev, opts|
        cur_sev, cur_opts = rules[id] || [:off, {}]
        rules[id] = [sev || (cur_sev == :off ? Config.on_severity(id) : cur_sev), cur_opts.merge(opts || {})]
      end
      (@cli[:disable] || []).each { |id| rules[id] = [:off, (rules[id] || [nil, {}])[1]] }
      if (only = @cli[:only]) && !only.empty?
        rules.each_key { |id| rules[id] = [:off, rules[id][1]] unless only.include?(id) }
        only.each do |id|
          s, o = rules[id]
          rules[id] = [s.nil? || s == :off ? Config.on_severity(id) : s, o || {}]
        end
      end
      return unless @cli[:quiet]

      rules.each_key { |id| rules[id] = [:off, rules[id][1]] unless rules[id][0] == :error }
    end

    # The configuration for one file, with matching overrides applied.
    def for_file(file)
      rel = relative(file)
      matching = @overrides.select do |o|
        raise ConfigError, "#{where}: each override needs files: and rules:" unless o.is_a?(Hash) && o["files"]

        Array(o["files"]).any? { |g| File.fnmatch?(g, rel, File::FNM_PATHNAME | File::FNM_EXTGLOB) }
      end
      return self if matching.empty?

      c = dup
      rules = @rules
      matching.each { |o| rules = merge_rules(rules, o["rules"] || {}, "#{where} (override #{Array(o['files']).join(', ')})") }
      c.instance_variable_set(:@rules, rules)
      cli_rules(rules)
      c
    end

    def relative(file)
      abs = File.expand_path(file)
      abs.start_with?("#{@dir}/") ? abs.delete_prefix("#{@dir}/") : file
    end

    def excluded?(file)
      rel = relative(file)
      @exclude.any? { |g| File.fnmatch?(g, rel, File::FNM_PATHNAME | File::FNM_EXTGLOB) || rel.start_with?("#{g.chomp('/**').chomp('/')}/") }
    end

    def included?(file)
      rel = relative(file)
      @include.any? { |g| File.fnmatch?(g, rel, File::FNM_PATHNAME | File::FNM_EXTGLOB) || File.fnmatch?(g, File.basename(rel)) }
    end

    def rule(id) = @rules[id] || [:off, {}]

    def enabled_rules = @rules.select { |_, (s, _)| s != :off }.keys

    def rules = @rules

    # Names defined in the desktop's init.rb, which runs before every buffer.
    def init_definitions
      return Set.new unless @use_init

      @init_definitions ||= begin
        home = ENV["SONIC_PI_HOME"] || Dir.home
        p = File.join(home, ".sonic-pi", "config", "init.rb")
        names = Set.new
        if File.file?(p)
          sexp = Ripper.sexp(File.read(p))
          if sexp
            AST.each_call(sexp) do |c, _|
              l = c.arg_lit(0)
              names << l.value.to_s if c.fcall? && Rules::DEFINE_FNS.include?(c.sym) && l&.name?
            end
            AST.each_node(sexp) { |n| names << AST.ident(n[1]) if n[0] == :def && AST.ident(n[1]) }
          end
        end
        names
      rescue StandardError
        Set.new
      end
    end

    # The rc file `sonicpi lint --init` writes: every rule, with its default.
    def self.template
      by_cat = Rules.all.values.group_by(&:category)
      out = +<<~YAML
        # .splintrc - configuration for `sonicpi lint`
        #
        # Severities: error, warning, info, off. A rule can also take options:
        #   line-length: { severity: warning, max: 100 }
        # Disable a rule on one line with a comment:  # splint:disable-line unknown-opt
        # Every rule: sonicpi lint --list-rules     One rule: sonicpi lint --explain RULE

        # Start from a preset: recommended, strict, all or none.
        extends: recommended

        # Exit with an error when there is a problem at this level or worse:
        # error, warning, info or never.
        fail_on: error

        # Also fail when there are more warnings than this (-1: no limit).
        max_warnings: -1

        # Output: pretty, compact, json, github, sarif or checkstyle.
        format: pretty

        # Which files `sonicpi lint DIR` picks up, relative to this file.
        include:
          - "**/*.rb"
        exclude: []

        # Functions defined elsewhere (another buffer, run_file), so they are
        # not reported as undefined.
        globals: []

        # Treat the functions in ~/.sonic-pi/config/init.rb as defined.
        use_init: true

        # Honour # splint:disable comments, and report ones that do nothing.
        inline_config: true
        report_unused_disable: false

        rules:
      YAML
      %i[correctness suspicious performance compat style].each do |cat|
        next unless by_cat[cat]

        out << "  # #{cat}\n"
        by_cat[cat].each do |k|
          v = k.options.empty? ? k.severity.to_s : "{ severity: #{k.severity}, #{k.options.map { |o, d| "#{o}: #{d.inspect}" }.join(', ')} }"
          entry = "  #{k.id}: #{v}"
          out << "#{entry}#{' ' * [1, 34 - entry.length].max}# #{k.summary}\n"
        end
      end
      out << <<~YAML

        # Different settings for some files:
        overrides: []
        #  - files: ["sketches/**"]
        #    rules:
        #      undefined-function: off
      YAML
      out
    end
  end

  def self.suggest_rule(id)
    near = Rules.all.keys.min_by { |r| distance(id, r) }
    near && distance(id, near) <= 3 ? " (did you mean #{near}?)" : " (see sonicpi lint --list-rules)"
  end
end
