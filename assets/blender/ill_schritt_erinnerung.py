# Schritt "Erinnerung": grosses Handy mit Erinnerungs-Karte, Glocke, Kammi bringt den Briefumschlag.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzprops as KR
from kzlib import SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    ph = K.empty("Handy", loc=(-0.25, 0.25, 0), rot=(0, 0, R(10)))
    d = yield from KR.big_phone("Handy", ph, w=0.66, h=1.28, body=NIGHT, screen=WHITE)
    K.shadow("Schatten_Handy", -0.25, 0.22, 0.42, 0.12)
    # Erinnerungs-Karte auf dem Display
    card_y = 0.86
    K.slab("Karte", K.rrect(0.54, 0.2, 0.04, 8, 0, card_y), 0.003, 0.006, parent=d, mat=K.flat(SKY_L), line=False)
    K.slab("Karte_Icon", K.ellipse(-0.19, card_y, 0.055, 0.055, 40), 0.0062, 0.0075, parent=d, mat=K.flat(YELLOW), line=False)
    bell_pts = [(-0.215, card_y - 0.025), (-0.165, card_y - 0.025), (-0.17, card_y + 0.01), (-0.19, card_y + 0.03),
                (-0.21, card_y + 0.01)]
    K.slab("Karte_Glocke", bell_pts, 0.0076, 0.0085, parent=d, mat=K.flat(NIGHT), line=False)
    K.text("Karte_Titel", K.say("Erinnerung", "Reminder"), 0.042, parent=d, loc=(-0.115, card_y + 0.012, 0.0062), mat=K.flat(NIGHT))
    K.text("Karte_Zeit", K.say("Morgen, 10:30 Uhr", "Tomorrow, 10:30"), 0.034, font="Roboto-Medium.ttf", parent=d,
           loc=(-0.115, card_y - 0.05, 0.0062), mat=K.flat(K.mix_hex(NIGHT, WHITE, 0.25)))
    log_lines = [(0.0, 0.62, 0.46), (0.0, 0.56, 0.34), (0.0, 0.5, 0.4)]
    for i, (x, y, w) in enumerate(log_lines):
        K.slab("Zeile_%d" % i, K.rrect(w, 0.028, 0.014, 6, -0.23 + w / 2, y), 0.003, 0.005, parent=d,
               mat=K.flat(K.mix_hex(NIGHT, WHITE, 0.86)), line=False)
    K.slab("Knopf", K.rrect(0.42, 0.085, 0.0425, 10, 0, 0.3), 0.003, 0.006, parent=d, mat=K.flat(YELLOW), line=False)
    K.text("Knopf_Text", K.say("Termin ansehen", "View appointment"), 0.034, align="CENTER", parent=d, loc=(0, 0.288, 0.0062), mat=K.flat(NIGHT))
    yield
    # Glocke links neben dem Handy
    bl = K.empty("Glocke", loc=(-0.95, 0.05, 0), rot=(0, 0, R(-12)))
    yield from KR.bell("Glocke", bl, color=YELLOW, size=1.0)
    K.shadow("Schatten_Glocke", -0.95, 0.05, 0.24, 0.1)
    # Kammi vorne rechts mit Umschlag
    km = K.kammi("Kammi", loc=(0.66, -0.3, 0), rot_z=-24, scale=1.25,
                 pose={"arm_l": (0.75, -0.2, 0.72), "arm_r": (-0.35, -0.7, 0.62), "lean": (0, -4, 0)})
    K.shadow("Schatten_Kammi", 0.66, -0.3, 0.32, 0.15)
    yield
    env = KR.envelope("Umschlag", None)
    K.kammi_hold(km, env, "R", offset=(0, -0.03, 0.08), rot=(78, -14, -18))
    ex, ez = env.matrix_world.translation.x, env.matrix_world.translation.z
    # Flugspur vom Umschlag zum Handy (Deko-Punkte)
    pts = K.bez2((ex - 0.12, ez + 0.12), (ex - 0.3, ez + 0.55), (-0.05, 1.2), 9)
    for i, (x, z) in enumerate(pts[1:-1]):
        K.slab("Spur_%d" % i, K.ellipse(0, 0, 0.012, 0.012, 16), 0.0, 0.002, mat=K.flat(CORAL), line=False,
               loc=(x, -0.36, z), rot=(R(90), 0, 0))
    yield


K.run("schritt-erinnerung", build, (1200, 900), samples=24, frame_fill=(0.88, 0.88), cam=(-90, 8))
