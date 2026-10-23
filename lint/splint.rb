# frozen_string_literal: true

# splint - the Sonic Pi linter and formatter behind `sonicpi lint` and
# `sonicpi fmt`. It reads Sonic Pi's own synths, FX, samples and language
# from the bundled Sonic Pi server, so it always matches that version.
#
#   ruby splint.rb --sonic-pi-root DIR lint [options] [FILE|DIR ...]
#   ruby splint.rb --sonic-pi-root DIR fmt  [options] FILE ...

require "optparse"

Encoding.default_external = Encoding::UTF_8
Encoding.default_internal = nil
require "set"

$LOAD_PATH.unshift File.join(__dir__, "lib")
require "splint/ast"
require "splint/knowledge"
require "splint/rules"
require "splint/linter"
require "splint/config"
require "splint/report"

module Splint
  EXIT_OK = 0
  EXIT_PROBLEMS = 1
  EXIT_USAGE = 2

  class CLI
    def initialize(argv)
      @argv = argv.dup
      @root = nil
      @color = false
    end

    def run
      take_globals
      cmd = @argv.first && !@argv.first.start_with?("-") && %w[lint fmt].include?(@argv.first) ? @argv.shift : "lint"
      cmd == "fmt" ? fmt : lint
    rescue ConfigError => e
      warn "sonicpi lint: #{e.message}"
      EXIT_USAGE
    rescue OptionParser::ParseError => e
      warn "sonicpi #{cmd || 'lint'}: #{e.message}"
      EXIT_USAGE
    rescue Interrupt
      130
    end

    def take_globals
      if (i = @argv.index("--sonic-pi-root"))
        @root = @argv[i + 1]
        @argv.slice!(i, 2)
      end
      @root ||= ENV["SONICPI_ROOT"] || File.expand_path("../external/sonic-pi", __dir__)
      if (i = @argv.index("--color"))
        @color = true
        @argv.delete_at(i)
      end
      @color = false if @argv.delete("--no-color")
    end

    def knowledge
      @knowledge ||= Knowledge.load(@root)
    rescue ArgumentError => e
      raise ConfigError, e.message
    end

    # ---- lint -------------------------------------------------------------

    def lint
      o = { rules: [], disable: [], only: [] }
      files = nil
      op = OptionParser.new do |p|
        p.banner = "Usage: sonicpi lint [options] [FILE|DIR ...]   (default: the current directory)"
        p.separator ""
        p.separator "A .splintrc is required: the nearest one above each file is used."
        p.separator "Create one with `sonicpi lint --init`."
        p.separator ""
        p.separator "Configuration:"
        p.on("-c", "--config FILE", "use FILE instead of the nearest .splintrc") { |v| o[:config] = v }
        p.on("--init", "write a .splintrc listing every rule, and exit") { o[:init] = true }
        p.on("--force", "with --init: overwrite an existing .splintrc") { o[:force] = true }
        p.on("--use-defaults", "lint with the recommended preset when there is no .splintrc") { o[:defaults] = true }
        p.on("--print-config FILE", "print the effective configuration for FILE, and exit") { |v| o[:print_config] = v }
        p.on("--preset NAME", PRESETS, "start from a preset instead of the config's extends (#{PRESETS.join(', ')})") { |v| o[:preset] = v }
        p.separator ""
        p.separator "Rules:"
        p.on("--rule RULE[=SEVERITY][,OPT=VAL...]", "turn a rule on or set it, e.g. --rule line-length=warning,max=100") { |v| o[:rules] << parse_rule_flag(v) }
        p.on("--disable RULE", "turn a rule off (repeatable, or comma-separated)") { |v| o[:disable].concat(v.split(",")) }
        p.on("--only RULE", "run only these rules (repeatable, or comma-separated)") { |v| o[:only].concat(v.split(",")) }
        p.on("--no-inline-config", "ignore # splint: comments") { o[:inline] = false }
        p.on("--report-unused-disable", "report # splint:disable comments that suppress nothing") { o[:report_unused_disable] = true }
        p.on("--list-rules", "list every rule with its default severity, and exit") { o[:list] = true }
        p.on("--explain RULE", "describe a rule, and exit") { |v| o[:explain] = v }
        p.separator ""
        p.separator "Output:"
        p.on("-f", "--format NAME", FORMATS, "output format: #{FORMATS.join(', ')}") { |v| o[:format] = v }
        p.on("-o", "--output FILE", "write the report to FILE") { |v| o[:output] = v }
        p.on("-q", "--quiet", "report errors only") { o[:quiet] = true }
        p.on("--stats", "count problems by rule") { o[:stats] = true }
        p.separator ""
        p.separator "Exit status:"
        p.on("--fail-on LEVEL", FAIL_ON, "fail on problems at LEVEL or worse: #{FAIL_ON.join(', ')}") { |v| o[:fail_on] = v }
        p.on("--max-warnings N", Integer, "fail when there are more than N warnings") { |v| o[:max_warnings] = v }
        p.on("--exit-zero", "always exit 0 (unless the configuration is invalid)") { o[:exit_zero] = true }
        p.separator ""
        p.separator "Fixing:"
        p.on("--fix", "fix what can be fixed (whitespace, tabs, final newline, indentation) in place") { o[:fix] = true }
        p.on("--fix-dry-run", "show what --fix would change, without writing") { o[:fix_dry] = true }
        p.separator ""
        p.separator "Input:"
        p.on("--stdin", "lint code from standard input") { o[:stdin] = true }
        p.on("--stdin-filename NAME", "the file name to report (and find the config) for --stdin") { |v| o[:stdin_name] = v }
        p.on("-w", "--watch", "lint again whenever a file changes") { o[:watch] = true }
        p.on("--version", "print the Sonic Pi version the rules come from") { o[:version] = true }
        p.on("-h", "--help", "show this help") do
          puts p
          return EXIT_OK
        end
      end
      files = op.parse(@argv)

      return list_rules if o[:list]
      return explain(o[:explain]) if o[:explain]
      return init_rc(o[:force]) if o[:init]

      if o[:version]
        puts "splint, rules from Sonic Pi #{knowledge.version}"
        return EXIT_OK
      end
      validate_rule_ids!(o)

      if o[:print_config]
        cfg = config_for(o[:print_config], o).for_file(o[:print_config])
        puts "# #{cfg.path || 'defaults'}"
        cfg.rules.sort.each { |id, (s, opts)| puts "#{id}: #{s}#{opts.empty? ? '' : " #{opts}"}" }
        return EXIT_OK
      end

      loop do
        status = lint_once(files, o)
        return status unless o[:watch]

        wait_for_change(files)
      end
    end

    def validate_rule_ids!(o)
      (o[:rules].map(&:first) + o[:disable] + o[:only]).each do |id|
        raise ConfigError, "unknown rule #{id.inspect}#{Splint.suggest_rule(id)}" unless Rules.all.key?(id)
      end
    end

    def parse_rule_flag(v)
      id, rest = v.split("=", 2)
      return [id, nil, {}] unless rest

      parts = rest.split(",")
      sev = nil
      opts = {}
      parts.each do |p|
        if p.include?("=")
          k, val = p.split("=", 2)
          opts[k] = typed(val)
        else
          sev = Splint.severity(p)
        end
      end
      [id, sev, opts]
    end

    def typed(v)
      return v.to_i if v.match?(/\A-?\d+\z/)
      return v.to_f if v.match?(/\A-?\d+\.\d+\z/)
      return true if v == "true"
      return false if v == "false"

      v.include?(":") ? v.split(":") : v
    end

    def cli_overrides(o)
      o.slice(:preset, :rules, :disable, :only, :fail_on, :max_warnings, :format, :inline, :report_unused_disable, :quiet)
    end

    def config_for(file, o)
      @configs ||= {}
      path = o[:config] || Config.find(File.dirname(File.expand_path(file)))
      if path.nil?
        return @configs[:defaults] ||= Config.defaults(cli_overrides(o)) if o[:defaults]

        raise ConfigError, "no .splintrc found for #{file} (looked in #{File.dirname(File.expand_path(file))} and above).\n" \
                           "Create one with `sonicpi lint --init`, or pass --config FILE or --use-defaults."
      end
      @configs[path] ||= Config.load(path, cli_overrides(o))
    end

    def expand(files, o)
      files = ["."] if files.empty? && !o[:stdin]
      out = []
      files.each do |f|
        if File.directory?(f)
          cfg = config_for(File.join(f, "x.rb"), o)
          Dir.glob(File.join(f, "**", "*"), File::FNM_DOTMATCH).sort.each do |p|
            next unless File.file?(p)
            next if p.split(File::SEPARATOR).any? { |s| %w[.git node_modules].include?(s) }
            next unless cfg.included?(p) && !cfg.excluded?(p)

            out << p.delete_prefix("./")
          end
        elsif File.file?(f)
          out << f
        else
          raise ConfigError, "no such file or directory: #{f}"
        end
      end
      out
    end

    def lint_once(files, o)
      linter = Linter.new(knowledge)
      inputs = if o[:stdin]
                 [[o[:stdin_name] || "stdin.rb", $stdin.read]]
               else
                 expand(files, o).map { |f| [f, File.read(f, mode: "rb").force_encoding("UTF-8")] }
               end
      all = []
      sources = {}
      fail_on = nil
      max_warnings = -1
      format = o[:format]
      inputs.each do |name, src|
        cfg = config_for(name, o).for_file(name)
        fail_on ||= cfg.fail_on
        max_warnings = cfg.max_warnings if max_warnings.negative?
        format ||= cfg.format
        if o[:fix] || o[:fix_dry]
          fixed = linter.fix(name, src, cfg)
          if fixed != src
            if o[:fix_dry] || o[:stdin]
              puts diff(name, src, fixed)
            else
              File.write(name, fixed)
            end
            src = fixed
          end
        end
        sources[name] = src
        all.concat(linter.lint(name, src, cfg, report_unused_disable: cfg.report_unused_disable))
      end
      format ||= "pretty"
      text = Report.render(format, all, sources, color: @color && !o[:output], stats: o[:stats])
      if o[:output]
        File.write(o[:output], text)
      else
        $stdout.write(text)
      end
      return EXIT_OK if o[:exit_zero]

      failing(all, fail_on || "error", max_warnings) ? EXIT_PROBLEMS : EXIT_OK
    end

    def failing(diags, fail_on, max_warnings)
      return true if max_warnings >= 0 && diags.count { |d| d.severity == :warning } > max_warnings
      return false if fail_on == "never"

      floor = Splint.rank(fail_on.to_sym)
      diags.any? { |d| Splint.rank(d.severity) >= floor }
    end

    def wait_for_change(files)
      watched = (files.empty? ? ["."] : files).flat_map { |f| File.directory?(f) ? Dir.glob(File.join(f, "**", "*.rb")) : [f] }
      stamp = ->(fs) { fs.map { |f| File.exist?(f) ? File.mtime(f) : nil } }
      before = stamp.call(watched)
      $stderr.puts "\nWatching #{watched.size} #{watched.size == 1 ? 'file' : 'files'} (Ctrl-C to stop)..."
      sleep 0.3 while stamp.call(watched) == before
      $stdout.write("\e[2J\e[H") if @color
    end

    def diff(name, a, b)
      al = a.split("\n", -1)
      bl = b.split("\n", -1)
      out = +"--- #{name}\n+++ #{name} (fixed)\n"
      [al.size, bl.size].max.times do |i|
        next if al[i] == bl[i]

        if al[i].nil? && bl[i] == "" && i == bl.size - 1
          out << "@@ end of file @@\n+(newline at end of file)\n"
          next
        end
        out << "@@ line #{i + 1} @@\n"
        out << "-#{al[i]}\n" if al[i]
        out << "+#{bl[i]}\n" if bl[i]
      end
      out
    end

    def list_rules
      puts format("%-22s %-9s %-12s %s", "RULE", "DEFAULT", "CATEGORY", "WHAT IT CHECKS")
      Rules.all.values.sort_by { |k| [%i[correctness suspicious performance compat style].index(k.category), k.id] }.each do |k|
        fix = k.fixable? ? " (fixable)" : ""
        puts format("%-22s %-9s %-12s %s%s", k.id, k.severity, k.category, k.summary, fix)
      end
      puts "\nPresets: recommended (the defaults above), strict, all, none. Rules come from Sonic Pi #{knowledge.version}."
      EXIT_OK
    end

    def explain(id)
      k = Rules.all[id]
      raise ConfigError, "unknown rule #{id.inspect}#{Splint.suggest_rule(id)}" unless k

      puts "#{k.id} (#{k.category}, default: #{k.severity}#{k.fixable? ? ', fixable' : ''})"
      puts
      puts k.summary.sub(/\A./, &:upcase)
      unless k.details.empty?
        puts
        puts k.details
      end
      unless k.options.empty?
        puts
        puts "Options:"
        k.options.each { |n, d| puts "  #{n}: #{d.inspect}" }
      end
      puts
      puts "In .splintrc:  #{k.id}: #{k.options.empty? ? 'warning' : "{ severity: warning, #{k.options.map { |n, d| "#{n}: #{d.inspect}" }.join(', ')} }"}"
      puts "On one line:   # splint:disable-line #{k.id}"
      EXIT_OK
    end

    def init_rc(force)
      path = ".splintrc"
      if File.exist?(path) && !force
        warn "sonicpi lint: #{path} already exists (use --force to overwrite)"
        return EXIT_USAGE
      end
      File.write(path, Config.template)
      puts "Wrote #{path}. Run `sonicpi lint` to check the files here."
      EXIT_OK
    end

    # ---- fmt --------------------------------------------------------------

    def fmt
      o = {}
      op = OptionParser.new do |p|
        p.banner = "Usage: sonicpi fmt [options] FILE... (or - for stdin)\n\n" \
                   "Auto-aligns code the way the desktop's Run (and Alt-M) does, using\n" \
                   "Sonic Pi's own beautifier. Prints the result unless -w is given."
        p.on("-w", "--write", "rewrite the files in place") { o[:write] = true }
        p.on("--check", "change nothing; exit 1 if any file is not aligned") { o[:check] = true }
        p.on("-d", "--diff", "show what would change") { o[:diff] = true }
        p.on("-h", "--help", "show this help") do
          puts p
          return EXIT_OK
        end
      end
      files = op.parse(@argv)
      raise OptionParser::MissingArgument, "files to format" if files.empty?

      unaligned = 0
      broken = 0
      files.each do |f|
        src = f == "-" ? $stdin.read : File.read(f)
        sexp_ok = Syntax.check(src, f).empty?
        unless sexp_ok
          warn "sonicpi fmt: #{f}: not formatted, it has a syntax error (sonicpi lint #{f} shows it)"
          broken += 1
          next
        end
        out = Format.beautify(knowledge, src)
        changed = out != src
        unaligned += 1 if changed
        if o[:check]
          puts "#{f}: not aligned" if changed
        elsif o[:diff]
          puts diff(f, src, out) if changed
        elsif o[:write] && f != "-"
          File.write(f, out) if changed
        else
          $stdout.write(out)
        end
      end
      broken.positive? || (o[:check] && unaligned.positive?) ? EXIT_PROBLEMS : EXIT_OK
    end
  end
end

exit Splint::CLI.new(ARGV).run if $PROGRAM_NAME == __FILE__
