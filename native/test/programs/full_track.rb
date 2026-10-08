# Full-track arrangement
# Based on the beat by Pit Noack (SonicPit), from "Beats basteln wie die Großen", c't 13/2017
# Arrangement, section system, sine-pluck arp, risers and snare rolls added on top.
#
# Note: requires a powerful machine to run smoothly.
# ~4:10 at 100 BPM (104 bars). Every loop counts its own bars, so all parts stay
# in sync deterministically and stop on their own at the end.

use_bpm 100
use_debug false
use_sched_ahead_time 1

# ===================== STRUCTURE =====================
# [first bar, section]   1 bar = 4 beats = 2.4 s
structure = [
  [0,   :intro],      # pads fade in over vinyl hiss, soft hats
  [8,   :verse],      # kick comes in
  [24,  :build],      # snare, filtered arp opening up, riser, snare roll
  [32,  :drop],       # everything
  [56,  :breakdown],  # drums out, arp + pads float, riser into...
  [72,  :drop],       # second drop
  [96,  :outro],      # pads fade out over hiss and hats
  [104, :done]
]

layers = {
  intro:     [:pads, :hiss, :hats],
  verse:     [:pads, :hats, :kick],
  build:     [:pads, :hats, :kick, :snare, :arp],
  drop:      [:pads, :hats, :kick, :snare, :arp],
  breakdown: [:pads, :arp, :hiss],
  outro:     [:pads, :hiss, :hats],
  done:      []
}

define :section_at do |bar|
  structure.select { |start, _| start <= bar }.last[1]
end

define :section_start do |bar|
  structure.select { |start, _| start <= bar }.last[0]
end

define :active do |layer, bar|
  layers[section_at(bar)].include?(layer)
end

# ===================== CHORDS =====================
# Same progression as the original: C, Eb, B, D (maj9), as 4 + 4 + 8 + 8 bars = 24-bar cycle.
# Computed from the bar number instead of a shared variable, so there are no timing races.
chord_1 = chord :c4, :maj9, num_octaves: 2
chord_2 = chord :es4, :maj9, num_octaves: 2
chord_3 = chord :b3, :maj9, num_octaves: 2
chord_4 = chord :d4, :maj9, num_octaves: 2

chord_low_1 = chord :c2, :maj9
chord_low_2 = chord :es2, :maj9
chord_low_3 = chord :b1, :maj9
chord_low_4 = chord :d2, :maj9

define :chord_index do |bar|
  b = bar % 24
  if b < 4 then 0 elsif b < 8 then 1 elsif b < 16 then 2 else 3 end
end

define :chord_high_at do |bar|
  [chord_1, chord_2, chord_3, chord_4][chord_index(bar)]
end

define :chord_low_at do |bar|
  [chord_low_1, chord_low_2, chord_low_3, chord_low_4][chord_index(bar)]
end

# ===================== HISS =====================
live_loop :hiss_loop do
  bar = tick
  stop if section_at(bar) == :done
  if active(:hiss, bar)
    sample :vinyl_hiss, amp: 1.5, start: rand(0.5), attack: 0.2, sustain: 3.6, release: 0.4
  end
  sleep 4
end

# ===================== HIHAT =====================
define :hihat do |amp|
  use_synth :dpulse
  with_fx :hpf, cutoff: 120 do
    play release: 0.01, amp: 13 * amp
  end
end

live_loop :hihat_loop do
  bar = tick(:bar)
  stop if section_at(bar) == :done
  hat_amp = { intro: 0.35, outro: 0.5 }.fetch(section_at(bar), 1.0)
  divisors = ring 2, 4, 2, 2, 2, 2, 2, 6
  4.times do
    d = divisors.tick(:div)
    d.times do
      hihat hat_amp if active(:hats, bar)
      sleep 1.0 / d
    end
  end
end

# ===================== SNARE =====================
live_loop :snare_loop do
  bar = tick
  stop if section_at(bar) == :done
  pos = bar - section_start(bar)

  if section_at(bar) == :build && pos >= 6
    # Snare roll over the last 2 bars of the build: 8ths, then 16ths, getting louder
    n = pos == 6 ? 8 : 16
    n.times do |i|
      progress = ((pos - 6) * 4 + i * (4.0 / n)) / 8.0
      sample :drum_snare_hard, lpf: 100, sustain: 0, release: 0.05, amp: 0.6 + 2.4 * progress
      sleep 4.0 / n
    end
  else
    s = ring(2.5, 3)[bar]
    sleep s
    sample :drum_snare_hard, lpf: 100, sustain: 0, release: 0.05, amp: 3 if active(:snare, bar)
    sleep 4 - s
  end
end

# ===================== BASSDRUM =====================
define :bassdrum do |note1, duration, note2 = note1|
  use_synth :sine
  with_fx :hpf, cutoff: 100 do
    play note1 + 24, amp: 40, release: 0.01
  end
  with_fx :distortion, distort: 0.1, mix: 0.3 do
    with_fx :lpf, cutoff: 26 do
      with_fx :hpf, cutoff: 55 do
        bass = play note1, amp: 85, release: duration, note_slide: duration
        control bass, note: note2
      end
    end
  end
  sleep duration
end

live_loop :bassdrum_schleife do
  bar = tick
  stop if section_at(bar) == :done
  if active(:kick, bar)
    bassdrum 36, 1.5
    if bools(0,0,0,0,0,0,0,0,0,1,0,0,0,0,0,0)[bar]
      bassdrum 36, 0.5, 40
      bassdrum 38, 1, 10
    else
      bassdrum 36, 1.5
    end
    bassdrum 36, 1.0, ring(10, 10, 10, 40)[bar]
  else
    sleep 4
  end
end

# ===================== SPHERES (PADS) =====================
define :chord_player do |the_chord, amp|
  use_synth :blade
  the_chord.each do |note|
    play note, attack: rand(4), release: rand(6..8), cutoff: rand(50..85), vibrato_rate: rand(0.01..2), amp: amp
  end
end

define :pad_amp do |bar|
  case section_at(bar)
  when :intro     then 0.55 * [(bar + 2) / 8.0, 1.0].min   # fade in
  when :outro     then 0.55 * [(104 - bar) / 8.0, 0.0].max # fade out
  when :breakdown then 0.7                                  # pads take the spotlight
  else 0.55
  end
end

with_fx :reverb, room: 0.99, mix: 0.7 do
  live_loop :chord_loop do
    bar = tick * 2
    stop if section_at(bar) == :done
    if active(:pads, bar)
      a = pad_amp(bar)
      chord_player chord_high_at(bar).pick(6), a
      chord_player chord_low_at(bar).take(3), a
    end
    sleep 8
  end
end

# ===================== SINE-PLUCK ARP =====================
# The additive sine "guitar string": harmonics shaped by pluck position,
# high harmonics decaying faster than the fundamental.
define :pluck_note do |midi, vol, pan|
  f = midi_to_hz(midi)
  (1..4).each do |k|
    strength = Math.sin(k * Math::PI * 0.2).abs / (k ** 1.2)
    play hz_to_midi(k * f * Math.sqrt(1 + 0.0004 * k * k)),
      amp: vol * strength, pan: pan,
      attack: 0.003, sustain: 0, release: 0.9 / (k ** 0.8), env_curve: 2
  end
end

arp_shape = ring 0, 2, 4, 7, 5, 4, 2, 3   # indices into the 2-octave chord

with_fx :reverb, room: 0.7, mix: 0.35 do
  with_fx :echo, phase: 0.75, decay: 3, mix: 0.25 do
    live_loop :arp_loop do
      bar = tick
      stop if section_at(bar) == :done
      if active(:arp, bar)
        use_synth :sine
        sec = section_at(bar)
        notes = chord_high_at(bar)
        # In the build the filter opens bar by bar; in the breakdown it stays a bit softer
        cutoff = case sec
                 when :build     then 65 + 60 * (bar - 24) / 8.0
                 when :breakdown then 105
                 else 125
                 end
        vol = sec == :breakdown ? 0.4 : 0.5
        with_fx :lpf, cutoff: cutoff do
          8.times do |i|
            n = notes[arp_shape[i] + (bar / 2) % 2]   # shift the pattern every other 2 bars
            pluck_note n, vol * ring(1, 0.6, 0.8, 0.6)[i], ring(-0.4, 0.4)[i]
            sleep 0.5
          end
        end
      else
        sleep 4
      end
    end
  end
end

# ===================== RISERS & CRASHES =====================
with_fx :reverb, room: 0.8, mix: 0.4 do
  live_loop :fx_loop do
    bar = tick
    stop if section_at(bar) == :done

    # Sine risers: 8 bars into drop 1, 4 bars into drop 2
    if bar == 24 || bar == 68
      len = (bar == 24 ? 8 : 4) * 4
      use_synth :sine
      [0, 7, 12.1].each do |offset|
        r = play 48 + offset, amp: 0.2, attack: len - 0.5, sustain: 0.4, release: 0.1,
          note_slide: len, pan: rrand(-0.5, 0.5)
        control r, note: 96 + offset
      end
    end

    # Crash on each drop
    if bar == 32 || bar == 72
      sample :drum_cymbal_open, amp: 1.5
      sample :drum_splash_hard, amp: 0.8
    end

    sleep 4
  end
end

# Original beat coded by Pit Noack
# supported by
# Alexander Degraf
# Astrid Hagenguth
# Enrico Mercaldi
# http://www.maschinennah.de/
# mail@pitnoack.de
