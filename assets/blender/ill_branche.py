# Branchen-Szenen (1200x900, transparent): BRANCHE=friseur|barbershop|kosmetik|nagelstudio
# Je eine Person bei der Arbeit. Nutzt toon_lib (Look), kzlib/kzpeople/kzprops (Figuren, Moebel),
# damit alle Illustrationen gleich aussehen.
import sys, os, math
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
if not os.path.exists(os.path.join(HERE, "kzlib.py")) and os.environ.get("KZ_LIB_DIR"):
    sys.path.insert(0, os.environ["KZ_LIB_DIR"])
import bpy
from mathutils import Vector
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

T = K.T
R = math.radians
P = T.PAL
BRANCHE = os.environ.get("BRANCHE", "friseur")
OUT = os.path.join(K.ASSETS, "ill", "png", "branche-%s.png" % BRANCHE)


def wpos(o):
    bpy.context.view_layer.update()
    return o.matrix_world.translation.copy()


# ------------------------------------------------------------------ eigene Requisiten (toon_lib)

def round_brush(n, parent, loc=(0, 0, 0), rot=(0, 0, 0)):
    """Rundbuerste: Griff entlang -Z (liegt in der Hand), Borsten oben."""
    g = T.empty(n, loc=loc, rot=rot, parent=parent, size=0.02)
    T.capsule(n + "_Griff", (0, 0, 0.02), (0, 0, -0.1), 0.012, 0.01, color=NIGHT, parent=g)
    T.capsule(n + "_Walze", (0, 0, 0.03), (0, 0, 0.13), 0.022, 0.022, color=P["hellgrau"], parent=g)
    T.capsule(n + "_Borsten", (0, 0, 0.035), (0, 0, 0.125), 0.036, 0.036, color=CORAL, parent=g)
    return g


def barber_pole(n, loc):
    """Barber-Stange an der Wand: weisser Zylinder mit zwei Spiralstreifen, Kappen in Nachtblau."""
    g = T.empty(n, loc=loc, size=0.05)
    T.cylinder(n + "_Glas", (0, 0, 0), 0.065, 0.5, color=WHITE, parent=g)
    for k, c in enumerate((CORAL, BLUE)):
        pts = [Vector((0.066 * math.cos(2 * math.pi * (t / 40 * 3) + k * math.pi), 0.066 * math.sin(2 * math.pi * (t / 40 * 3) + k * math.pi),
                       0.02 + 0.46 * t / 40)) for t in range(41)]
        T.tube(n + "_Streifen%d" % k, pts, 0.014, color=c, parent=g, seg=10)
    for z, nm in ((-0.04, "Unten"), (0.5, "Oben")):
        T.cylinder(n + "_Kappe" + nm, (0, 0, z), 0.08, 0.05, color=NIGHT, parent=g, radius=0.01)
    T.ellipsoid(n + "_Knauf", (0, 0, 0.58), (0.04, 0.04, 0.04), color=YELLOW, parent=g)
    T.rbox(n + "_Halter", (0.04, 0.12, 0.03), loc=(0, 0.1, 0.25), radius=0.01, color=NIGHT, parent=g)
    return g


def razor(n, hand):
    """Rasiermesser in der Hand: Griff in der Faust, Klinge zeigt nach vorn-oben."""
    g = T.empty(n, loc=(0, -0.02, -0.05), parent=hand, size=0.02)
    g.scale = (1.6, 1.6, 1.6)
    T.rbox(n + "_Griff", (0.016, 0.012, 0.11), loc=(0, 0, -0.02), radius=0.005, color=NIGHT, parent=g)
    T.rbox(n + "_Klinge", (0.024, 0.004, 0.08), loc=(0.004, 0, 0.065), radius=0.0015, color=P["hellgrau"], steps=3,
           parent=g, rot=(0, -10, 0))
    return g


def mask_brush(n, hand):
    """Flacher Maskenpinsel in der Hand, Pinselspitze mit Maske (Mint)."""
    g = T.empty(n, loc=(0, -0.02, -0.05), parent=hand, size=0.02)
    T.capsule(n + "_Stiel", (0, 0, 0.04), (0, 0, -0.12), 0.007, 0.006, color=NIGHT, parent=g)
    T.rbox(n + "_Zwinge", (0.024, 0.008, 0.02), loc=(0, 0, 0.05), radius=0.003, color=P["hellgrau"], parent=g)
    T.ellipsoid(n + "_Spitze", (0, 0, 0.078), (0.016, 0.005, 0.026), color=P["mint_hell"], parent=g)
    return g


def jar(n, parent, loc, body=WHITE, lid=CORAL, cream=P["mint_hell"]):
    g = T.empty(n, loc=loc, parent=parent, size=0.03)
    T.revolve(n + "_Tiegel", [(0, 0), (0, 0.055), (0.005, 0.06), (0.05, 0.06), (0.055, 0.05), (0.055, 0)], color=body,
              parent=g)
    T.ellipsoid(n + "_Creme", (0, 0, 0.055), (0.05, 0.05, 0.012), color=cream, parent=g)
    T.cylinder(n + "_Deckel", (0.1, 0.02, 0), 0.062, 0.025, color=lid, parent=g, radius=0.006)
    return g


def polish(n, parent, loc, color, h=0.07):
    """Nagellack-Flasche: eckiges Glas mit Lackfarbe, hoher Deckel in Nachtblau."""
    g = T.empty(n, loc=loc, parent=parent, size=0.02)
    T.rbox(n + "_Glas", (0.04, 0.03, 0.045), loc=(0, 0, 0.0225), radius=0.008, color=color, steps=3, parent=g)
    T.rbox(n + "_Deckel", (0.02, 0.02, h - 0.045), loc=(0, 0, 0.045 + (h - 0.045) / 2), radius=0.005, color=NIGHT, parent=g)
    return g


def desk_lamp(n, parent, loc, shade=YELLOW):
    g = T.empty(n, loc=loc, parent=parent, size=0.03)
    T.cylinder(n + "_Fuss", (0, 0, 0), 0.07, 0.02, color=NIGHT, parent=g, radius=0.006)
    T.tube(n + "_Arm", [Vector((0, 0, 0.02)), Vector((0.02, 0, 0.22)), Vector((0.16, 0, 0.32))], 0.009, color=NIGHT, parent=g, seg=10)
    T.revolve(n + "_Schirm", [(0, 0), (0, 0.075), (0.02, 0.07), (0.08, 0.035), (0.085, 0)], color=shade, parent=g,
              loc=(0.18, 0, 0.36), rot=(0, 145, 0))
    return g


def stool(n, loc, seat=YELLOW, h=0.5):
    g = T.empty(n, loc=loc, size=0.04)
    T.cylinder(n + "_Sitz", (0, 0, h - 0.06), 0.2, 0.06, color=seat, parent=g, radius=0.02)
    T.cylinder(n + "_Saeule", (0, 0, 0.04), 0.025, h - 0.1, color=P["hellgrau"], parent=g)
    T.cylinder(n + "_Fuss", (0, 0, 0), 0.17, 0.04, color=P["hellgrau"], parent=g, radius=0.012)
    return g


def treatment_chair(n, loc, rot_z=0.0, color=WHITE, base=NIGHT):
    """Kosmetikliege halb aufgerichtet. Lokal: Kopfende +Y oben, Fussende -Y."""
    g = T.empty(n, loc=loc, rot=(0, 0, rot_z), size=0.05)
    T.rbox(n + "_Sockel", (0.42, 0.62, 0.36), loc=(0, 0.0, 0.18), radius=0.04, color=base, parent=g)
    T.rbox(n + "_Sitz", (0.6, 0.62, 0.12), loc=(0, 0.0, 0.42), radius=0.05, color=color, steps=2, parent=g)
    T.rbox(n + "_Lehne", (0.6, 0.12, 0.8), loc=(0, 0.48, 0.66), radius=0.05, color=color, steps=2, parent=g, rot=(-55, 0, 0))
    T.rbox(n + "_Beinteil", (0.6, 0.6, 0.11), loc=(0, -0.55, 0.36), radius=0.05, color=color, steps=2, parent=g, rot=(-12, 0, 0))
    T.rbox(n + "_Kissen", (0.36, 0.14, 0.12), loc=(0, 0.85, 0.98), radius=0.05, color=P["mint_hell"], parent=g, rot=(-55, 0, 0))
    return g


# ------------------------------------------------------------------ Szenen

def wall(color, w=3.6, h=2.35, x=0.0, y=0.95):
    g = KR.upright("Wand_Platz", None, loc=(x, y, 0))
    K.slab("Wand", K.rrect(w, h, 0.1, 8, 0, h / 2), 0, 0.06, parent=g, mat=K.toon(color))
    return g


def friseur():
    wall(T.mix(SKY, WHITE, 0.5))
    yield
    st = K.empty("Spiegelplatz", loc=(-1.05, 0.62, 0))
    yield from KR.mirror_station("Platz", st, frame=CORAL)
    K.shadow("Schatten_Platz", -1.05, 0.58, 0.62, 0.24)
    yield from KR.wall_shelf("Regal", K.empty("Regal_Platz", loc=(1.15, 0.91, 1.5)), bottles=(MINT, YELLOW, CORAL, SKY))
    pl = K.empty("Pflanze", loc=(1.55, 0.45, 0))
    yield from KR.plant("Pflanze", pl, pot=YELLOW, height=1.0)
    K.shadow("Schatten_Pflanze", 1.55, 0.45, 0.28, 0.14)
    ch = K.empty("Stuhl", loc=(0.0, 0.05, 0), rot=(0, 0, R(-10)))
    yield from KR.salon_chair("Stuhl", ch, seat=CORAL)
    K.shadow("Schatten_Stuhl", 0.0, 0.05, 0.5, 0.25)
    cl = K.empty("Kundin", loc=(0, 0.04, 0.57), parent=ch)
    jc = yield from KP.person("Kundin", cl,
                              dict(skin=SKIN[4], hair="afro", hair_color=P["haar_dunkel"], top=WHITE, bottom=BLUE,
                                   shoes=YELLOW, cape=MINT),
                              dict(KP.SIT, head=(2, 0, 6)))
    # Friseur mit Foehn und Rundbuerste
    sy = K.empty("Friseur", loc=(0.5, 0.32, 0.0), rot=(0, 0, R(-40)))
    j = yield from KP.person("Friseur", sy,
                             dict(skin=SKIN[1], hair="quiff", hair_color=P["haar_braun"], top=BLUE, bottom=NIGHT,
                                  shoes=WHITE, apron=None, sleeves="long"),
                             dict(arm_r=((-88, 2, 12), -30), arm_l=((-18, -10, 0), -50), head=(14, 0, 6),
                                  leg_r=((4, 0, 0), 0), leg_l=((-6, 0, 0), 5)))
    K.drop_to_floor(sy)
    head = wpos(jc["head"])
    yield from KR.hold_dryer("Foehn", j["hand_r"], head + Vector((0.0, 0.0, 0.08)), body=YELLOW)
    round_brush("Rundbuerste", j["hand_l"], loc=(0, -0.02, -0.05), rot=(90, 0, 0))
    K.shadow("Schatten_Friseur", 0.52, 0.32, 0.3, 0.15)
    yield


def barbershop():
    wall(T.mix(BLUE, WHITE, 0.62))
    yield
    st = K.empty("Spiegelplatz", loc=(1.05, 0.62, 0))
    yield from KR.mirror_station("Platz", st, frame=NIGHT)
    K.shadow("Schatten_Platz", 1.05, 0.58, 0.62, 0.24)
    barber_pole("Barberstange", (-1.25, 0.8, 1.05))
    yield
    yield from KR.wall_shelf("Regal", K.empty("Regal_Platz", loc=(-0.45, 0.91, 1.62)), w=0.56, bottles=(NIGHT, YELLOW, CORAL))
    ch = K.empty("Stuhl", loc=(0.0, 0.05, 0), rot=(0, 0, R(12)))
    yield from KR.salon_chair("Stuhl", ch, seat=NIGHT)
    K.shadow("Schatten_Stuhl", 0.0, 0.05, 0.5, 0.25)
    cl = K.empty("Kunde", loc=(0, 0.04, 0.57), parent=ch)
    jc = yield from KP.person("Kunde", cl,
                              dict(skin=SKIN[2], hair="beard", hair_color=P["haar_dunkel"], top=CORAL, bottom=NIGHT,
                                   shoes=NIGHT, cape=WHITE),
                              dict(KP.SIT, head=(-6, 0, -4)))
    sy = K.empty("Barbier", loc=(-0.5, 0.3, 0.0), rot=(0, 0, R(42)))
    j = yield from KP.person("Barbier", sy,
                             dict(skin=SKIN[3], hair="short", hair_color=NIGHT, top=WHITE, bottom=BLUE, shoes=NIGHT,
                                  apron=NIGHT, apron_low=-0.22),
                             dict(arm_r=((-96, 4, 16), -30), arm_l=((-80, -8, -16), -50), head=(16, 0, -6),
                                  leg_r=((-6, 0, 0), 4), leg_l=((4, 0, 0), 0)))
    K.drop_to_floor(sy)
    razor("Rasiermesser", j["hand_r"])
    K.shadow("Schatten_Barbier", -0.5, 0.3, 0.3, 0.15)
    yield


def kosmetik():
    wall(T.mix(MINT, WHITE, 0.66))
    yield
    pl = K.empty("Pflanze", loc=(1.5, 0.5, 0))
    yield from KR.plant("Pflanze", pl, pot=CORAL, height=1.1)
    K.shadow("Schatten_Pflanze", 1.5, 0.5, 0.28, 0.14)
    yield from KR.wall_shelf("Regal", K.empty("Regal_Platz", loc=(0.75, 0.91, 1.45)), w=0.6, bottles=(CORAL, WHITE, YELLOW))
    tb = K.empty("Tischchen", loc=(-1.25, 0.25, 0))
    yield from KR.side_table("Tischchen", tb, color=WHITE, top_h=0.62)
    jar("Tiegel", tb, (-0.03, -0.02, 0.62))
    K.shadow("Schatten_Tischchen", -1.25, 0.25, 0.24, 0.12)
    yield
    # Liege seitlich: Kopf links (-X), Fuesse rechts
    treatment_chair("Liege", (0.15, 0.1, 0), rot_z=90, color=SKY)
    K.shadow("Schatten_Liege", 0.15, 0.1, 0.85, 0.3)
    yield
    cl = K.empty("Kundin", loc=(0.12, 0.1, 0.5), rot=(0, 0, R(90)))
    jc = yield from KP.person("Kundin", cl,
                              dict(skin=SKIN[0], hair="bun", hair_color=P["haar_blond"], top=WHITE, bottom=BLUE,
                                   shoes=WHITE, sleeves="long"),
                              dict(leg_r=((-84, 0, -3), 10), leg_l=((-84, 0, 3), 10), torso=(-58, 0, 0),
                                   head=(-8, 0, 0), arm_r=((8, 14, 0), -35), arm_l=((8, -14, 0), -35)))
    # Maske mit Gurkenscheiben
    T.ellipsoid("Maske", (0, -0.095, 0.0), (0.085, 0.03, 0.095), color=P["mint_hell"], parent=jc["head"])
    for sx in (-1, 1):
        T.cylinder("Gurke_%d" % sx, (sx * 0.042, -0.122, 0.006), 0.026, 0.008, color=MINT, rot=(90, 0, 0),
                   parent=jc["head"], radius=0.003)
    yield
    sy = K.empty("Kosmetikerin", loc=(-0.72, 0.42, 0.0), rot=(0, 0, R(55)))
    j = yield from KP.person("Kosmetikerin", sy,
                             dict(skin=SKIN[2], hair="ponytail", hair_color=P["haar_dunkel"], top=CORAL, bottom=NIGHT,
                                  shoes=WHITE, sleeves="short"),
                             dict(arm_l=((-52, -4, -12), -28), arm_r=((-26, 6, 4), -70), head=(30, 0, -4), torso=(8, 0, 0),
                                  leg_r=((-4, 0, 0), 4), leg_l=((4, 0, 0), 0)))
    K.drop_to_floor(sy)
    mask_brush("Maskenpinsel", j["hand_l"])
    K.shadow("Schatten_Kosmetikerin", -0.72, 0.42, 0.3, 0.15)
    yield


def nagelstudio():
    wall(T.mix(CORAL, WHITE, 0.66))
    yield
    yield from KR.wall_shelf("Regal", K.empty("Regal_Platz", loc=(0.0, 0.91, 1.55)), w=1.1,
                             bottles=(CORAL, MINT, YELLOW, SKY, BLUE, CORAL))
    pl = K.empty("Pflanze", loc=(-1.5, 0.5, 0))
    yield from KR.plant("Pflanze", pl, pot=BLUE, height=0.95)
    K.shadow("Schatten_Pflanze", -1.5, 0.5, 0.28, 0.14)
    tb = K.empty("Nageltisch", loc=(0, 0.15, 0))
    yield from KR.desk("Nageltisch", tb, top=WHITE, legs=NIGHT, w=0.9, d=0.55, h=0.74)
    K.shadow("Schatten_Tisch", 0, 0.15, 0.55, 0.3)
    desk_lamp("Lampe", tb, (-0.3, 0.12, 0.74), shade=YELLOW)
    T.rbox("Handkissen", (0.2, 0.12, 0.05), loc=(0.12, 0.0, 0.765), radius=0.025, color=SKY, parent=tb)
    for i, (x, c) in enumerate(((-0.05, CORAL), (0.0, MINT), (0.05, YELLOW))):
        polish("Lack_%d" % i, tb, (x - 0.12, -0.12, 0.74), c)
    yield
    stool("Hocker_K", (0.72, 0.15, 0), seat=MINT)
    stool("Hocker_N", (-0.72, 0.15, 0), seat=YELLOW)
    K.shadow("Schatten_HK", 0.72, 0.15, 0.22, 0.12)
    K.shadow("Schatten_HN", -0.72, 0.15, 0.22, 0.12)
    yield
    cl = K.empty("Kundin", loc=(0.7, 0.15, 0.5), rot=(0, 0, R(-90)))
    yield from KP.person("Kundin", cl,
                         dict(skin=SKIN[3], hair="long", hair_color=P["haar_dunkel"], top=BLUE, bottom=NIGHT, shoes=CORAL),
                         dict(KP.SIT, arm_r=((-62, 4, -10), -14), arm_l=((-30, -4, 0), -60), head=(6, 0, 0)))
    sy = K.empty("Nageldesignerin", loc=(-0.7, 0.15, 0.5), rot=(0, 0, R(90)))
    j = yield from KP.person("Nageldesignerin", sy,
                             dict(skin=SKIN[0], hair="bob", hair_color=P["haar_rot"], top=MINT, bottom=BLUE, shoes=WHITE),
                             dict(KP.SIT, arm_r=((-66, 6, 6), -40), arm_l=((-62, -6, -4), -30), head=(18, 0, 0),
                                  torso=(8, 0, 0)))
    T.capsule("Nagelpinsel", (0, -0.02, -0.03), (0, -0.02, -0.1), 0.005, 0.004, color=CORAL, parent=j["hand_r"])
    yield


SCENES = {"friseur": friseur, "barbershop": barbershop, "kosmetik": kosmetik, "nagelstudio": nagelstudio}


def build():
    T.set_light(-130, 48)
    yield from SCENES[BRANCHE]()


T.Run("branche-" + BRANCHE, build, out=OUT, res=(1200, 900), samples=24, outline_px=2.4,
      frame_fill=(0.92, 0.9), cam=(-90, 9)).start()
