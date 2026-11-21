# Generates build/logo.svg and build/appicon.svg: python3 build/make_logo.py build
# Geometric malachi emblem: ring, six hexagon dots, M. Proportions measured
# from the potrace trace of the Gemini logo, made exact and symmetric.
import math, sys
outdir = sys.argv[1]
INK = "#122c47"
C, R, STROKE, DOT = 500, 400, 46, 54
k = 340 / 282                      # trace px -> logo units (M height 340)
half, diag, inner, vhalf = 166.5 * k, 105 * k, 99 * k, 19 * k
apex, itop, vbot, H = 158 * k, 120 * k, 242 * k, 282 * k
top = C - H / 2 + 6                # a touch low reads as centred
def P(dx, dy): return f"{C + dx:.1f},{top + dy:.1f}"
m = " ".join([P(-half, 0), P(-diag, 0), P(0, apex), P(diag, 0), P(half, 0), P(half, H),
              P(inner, H), P(inner, itop), P(vhalf, vbot), P(-vhalf, vbot), P(-inner, itop),
              P(-inner, H), P(-half, H)])
dots = "".join(
    f'<circle cx="{C + R * math.cos(math.radians(a)):.1f}" cy="{C - R * math.sin(math.radians(a)):.1f}" r="{DOT}"/>'
    for a in (90, 30, -30, -90, -150, 150))
emblem = (f'<g fill="{INK}"><circle cx="{C}" cy="{C}" r="{R}" fill="none" stroke="{INK}" stroke-width="{STROKE}"/>'
          f'{dots}<polygon points="{m}"/></g>')
open(f"{outdir}/logo.svg", "w").write(
    f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000" width="512" height="512">\n<!-- malachi emblem -->\n{emblem}\n</svg>\n')
N, inset, radius = 1024, 52, 210
tile = N - 2 * inset
size = tile * 0.78
x = (N - size) / 2
open(f"{outdir}/appicon.svg", "w").write(
    f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {N} {N}" width="{N}" height="{N}">\n'
    f'<rect x="{inset}" y="{inset}" width="{tile}" height="{tile}" rx="{radius}" fill="#f6f8fb"/>\n'
    f'<svg x="{x:.1f}" y="{x:.1f}" width="{size:.1f}" height="{size:.1f}" viewBox="0 0 1000 1000">{emblem}</svg>\n</svg>\n')
