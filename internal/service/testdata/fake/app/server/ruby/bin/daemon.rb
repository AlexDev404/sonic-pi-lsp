# A stand-in for Sonic Pi's boot daemon, for tests: it prints a Sonic Pi 5
# port line, answers /ping with /ack, and reports each /run-code as a run
# that logs one line and completes. It speaks just enough OSC.
require "socket"

def pad(s) = s + "\0" * (4 - s.bytesize % 4)

def osc(addr, *args)
  tags = "," + args.map { |a| a.is_a?(Integer) ? "i" : "s" }.join
  body = args.map { |a| a.is_a?(Integer) ? [a].pack("l>") : pad(a.b) }.join
  pad(addr.b) + pad(tags.b) + body
end

def parse(b)
  addr, rest = b.split("\0", 2)
  rest = rest.sub(/\A\0*/, "")
  tags, rest = rest.split("\0", 2)
  off = (addr.bytesize + 4) / 4 * 4 + (tags.bytesize + 4) / 4 * 4
  args = []
  tags.to_s[1..].to_s.each_char do |t|
    case t
    when "i" then args << b[off, 4].unpack1("l>"); off += 4
    when "f" then args << b[off, 4].unpack1("g"); off += 4
    when "s"
      s = b[off..].split("\0", 2)[0]
      args << s
      off += (s.bytesize + 4) / 4 * 4
    end
  end
  [addr, args]
end

def free_port
  s = UDPSocket.new
  s.bind("127.0.0.1", 0)
  p = s.addr[1]
  s.close
  p
end

daemon = UDPSocket.new; daemon.bind("127.0.0.1", 0)
spider = UDPSocket.new; spider.bind("127.0.0.1", 0)
gui_port = free_port
token = 4242
puts "#{daemon.addr[1]} #{gui_port} #{spider.addr[1]} #{free_port} #{free_port} #{token}"
$stdout.flush

gui = UDPSocket.new
send_gui = ->(addr, *args) { gui.send(osc(addr, *args), 0, "127.0.0.1", gui_port) }
runs = 0

Thread.new do
  loop do
    msg, = daemon.recvfrom(65536)
    addr, args = parse(msg)
    exit 0 if addr == "/daemon/exit" && args[0] == token
  end
end

loop do
  msg, = spider.recvfrom(65536)
  addr, args = parse(msg)
  next unless args[0] == token

  case addr
  when "/ping" then send_gui.call("/ack", args[1].to_s)
  when "/version" then send_gui.call("/version", "v5.0", 500, "v5.0", 500, 1, 1, 2026, "Test")
  when "/run-code"
    runs += 1
    code = args[1].to_s.lines.reject { |l| l.include?("#__nosave__") }.join.strip
    send_gui.call("/log/info", 0, "Starting run #{runs}")
    send_gui.call("/log/multi_message", runs, '""', "0.0", 1, 1, "ran: #{code}")
    send_gui.call("/error", runs, "boom", "trace", 2) if code.include?("boom")
    send_gui.call("/runs/all-completed")
  end
end
