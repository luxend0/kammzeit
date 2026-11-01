# Spot "Team": drei Mitarbeiterinnen/Mitarbeiter in ihren Kalenderfarben, daneben eine Tafel mit
# Arbeitszeiten (Balken mit Pause) in denselben Farben.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians
STAFF = [CORAL, MINT, YELLOW]


def hours_board(n, root, w=0.82, h=0.62):
    leg = K.toon(NIGHT)
    for sx in (-1, 1):
        K.capsule("%s_Bein_%d" % (n, sx), 1.22, 0.018, 0.018, parent=root, mat=leg, loc=(sx * 0.3, 0.1, 1.2),
                  rot=(R(-12), R(-sx * 5), 0))
    K.capsule(n + "_Bein_hinten", 1.18, 0.016, 0.016, parent=root, mat=leg, loc=(0, 0.12, 1.18), rot=(R(22), 0, 0))
    yield
    g = K.empty(n + "_Tafel", loc=(0, -0.09, 0.55), rot=(R(78), 0, 0), parent=root)
    K.slab(n + "_Flaeche", K.rrect(w, h, 0.04, 8, 0, h / 2), 0.0, 0.012, parent=g, mat=K.toon(WHITE))
    z = 0.0125
    grey = K.flat(K.mix_hex(NIGHT, WHITE, 0.86))
    x0, x1 = -w / 2 + 0.14, w / 2 - 0.05
    for i, t in enumerate(("9", "12", "15", "18")):
        x = x0 + (x1 - x0) * i / 3
        K.text("%s_Std_%d" % (n, i), t, 0.032, font="Roboto-Bold.ttf", align="CENTER", parent=g,
               loc=(x, h - 0.075, z), mat=K.flat(K.mix_hex(NIGHT, WHITE, 0.35)))
        K.slab("%s_Raster_%d" % (n, i), [(x - 0.0015, 0.05), (x + 0.0015, 0.05), (x + 0.0015, h - 0.09), (x - 0.0015, h - 0.09)],
               z, z + 0.001, parent=g, mat=grey, line=False)
    # (Start, Ende) Bloecke je Person, Luecke = Pause
    blocks = [((0.0, 0.42), (0.55, 1.0)), ((0.15, 0.5), (0.62, 0.92)), ((0.0, 0.3), (0.4, 0.75))]
    for r_, (col, segs) in enumerate(zip(STAFF, blocks)):
        y = h - 0.17 - r_ * 0.14
        K.slab("%s_Avatar_%d" % (n, r_), K.ellipse(-w / 2 + 0.065, y, 0.038, 0.038, 40), z, z + 0.002, parent=g,
               mat=K.flat(col), line=False)
        for k, (a, b) in enumerate(segs):
            xa, xb = x0 + (x1 - x0) * a, x0 + (x1 - x0) * b
            K.slab("%s_Balken_%d_%d" % (n, r_, k), K.rrect(xb - xa, 0.05, 0.025, 8, (xa + xb) / 2, y), z + 0.001, z + 0.003,
                   parent=g, mat=K.flat(col), line=False)
    K.log("Arbeitszeiten-Tafel")
    yield


def build():
    looks = [
        dict(skin=SKIN[0], hair="bob", hair_color=K.P["haar_braun"], top=STAFF[0], bottom=NIGHT, shoes=WHITE),
        dict(skin=SKIN[3], hair="beard", hair_color=K.P["haar_dunkel"], top=STAFF[1], bottom=BLUE, shoes=NIGHT,
             sleeves="long"),
        dict(skin=SKIN[4], hair="long", hair_color=NIGHT, top=STAFF[2], bottom=NIGHT, shoes=CORAL),
    ]
    poses = [
        dict(arm_r=((-20, 8, 0), -70), arm_l=((0, -10, 0), -10), head=(4, 0, 8), leg_r=((2, 0, 0), 0), leg_l=((-5, 0, 0), 4)),
        dict(arm_r=((0, 9, 0), -12), arm_l=((0, -9, 0), -12), head=(2, 0, 0), leg_r=((-3, 0, 0), 2), leg_l=((3, 0, 0), 0)),
        dict(arm_r=((0, 8, 0), -10), arm_l=((-30, -95, 0), -40), head=(4, 0, -8), leg_r=((4, 0, 0), 0), leg_l=((-4, 0, 0), 3)),
    ]
    xs = [-0.95, -0.32, 0.31]
    rots = [18, 4, -14]
    for i, (lk, ps) in enumerate(zip(looks, poses)):
        pe = K.empty("Person_%d" % i, loc=(xs[i], -0.05 + 0.08 * (i == 1), 0), rot=(0, 0, R(rots[i])))
        j = yield from KP.person("Person_%d" % i, pe, lk, ps)
        if i == 0:
            yield from KR.scissors("Schere", j["hand_r"], rings=MINT)
        if i == 1:
            yield from KR.comb("Kamm", j["hand_r"], color=NIGHT)
        K.drop_to_floor(pe)
        K.shadow("Schatten_Person_%d" % i, xs[i], -0.05 + 0.08 * (i == 1), 0.3, 0.15)
    bd = K.empty("Tafel", loc=(1.2, 0.15, 0), rot=(0, 0, R(-12)))
    yield from hours_board("Tafel", bd)
    K.shadow("Schatten_Tafel", 1.2, 0.2, 0.42, 0.14)
    yield


K.run("team", build, (1000, 800), samples=24, frame_fill=(0.88, 0.88), cam=(-90, 9))
