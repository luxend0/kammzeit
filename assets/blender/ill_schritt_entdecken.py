# Schritt "Salon entdecken": Person scrollt durch ein Social-Profil, Salonprofil mit Buchungsknopf.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def profile_screen(d, w, h):
    """Generisches Social-Profil eines Salons (keine Marke), mit Buchungsknopf."""
    grey = K.flat(K.mix_hex(NIGHT, WHITE, 0.86))
    top = h - 0.16
    K.slab("Profil_Bild", K.ellipse(-w / 2 + 0.13, top, 0.075, 0.075, 48), 0.003, 0.005, parent=d, mat=K.flat(CORAL), line=False)
    K.slab("Profil_Bild_Ring", K.ellipse(-w / 2 + 0.13, top, 0.085, 0.085, 48), 0.0025, 0.0028, parent=d,
           mat=K.flat(YELLOW), line=False)
    # kleine Schere als Profilbild
    for sx in (-1, 1):
        K.slab("Profil_Klinge_%d" % sx, K.band([(-w / 2 + 0.13, top - 0.02), (-w / 2 + 0.13 + sx * 0.03, top + 0.045)], 0.012, 0.006),
               0.0052, 0.006, parent=d, mat=K.flat(WHITE), line=False)
        K.slab("Profil_Ring_%d" % sx, K.ellipse(-w / 2 + 0.13 + sx * 0.022, top - 0.035, 0.014, 0.014, 24), 0.0052, 0.006,
               parent=d, mat=K.flat(WHITE), line=False)
    K.text("Profil_Name", "Salon Locke", 0.042, parent=d, loc=(-w / 2 + 0.24, top + 0.01, 0.004), mat=K.flat(NIGHT))
    K.slab("Profil_Zeile", K.rrect(0.2, 0.024, 0.012, 6, -w / 2 + 0.34, top - 0.04), 0.003, 0.005, parent=d, mat=grey, line=False)
    K.slab("Profil_Knopf", K.rrect(w - 0.1, 0.085, 0.0425, 10, 0, top - 0.15), 0.003, 0.006, parent=d, mat=K.flat(YELLOW), line=False)
    K.text("Profil_Knopf_Text", "Termin buchen", 0.036, align="CENTER", parent=d, loc=(0, top - 0.163, 0.0062), mat=K.flat(NIGHT))
    cols = [SKY, MINT, K.P["rosa"], YELLOW, BLUE, K.P["mint_hell"]]
    s = (w - 0.1 - 0.04) / 3
    for i, c in enumerate(cols):
        x = -w / 2 + 0.05 + s / 2 + (i % 3) * (s + 0.02)
        y = top - 0.27 - s / 2 - (i // 3) * (s + 0.02)
        K.slab("Raster_%d" % i, K.rrect(s, s, 0.02, 6, x, y), 0.003, 0.005, parent=d, mat=K.flat(c), line=False)
        # einfache Frisur-Silhouette pro Kachel
        K.slab("Raster_Kopf_%d" % i, K.ellipse(x, y - 0.01, s * 0.18, s * 0.21, 32), 0.0052, 0.006, parent=d,
               mat=K.flat(WHITE), line=False)
        K.slab("Raster_Haar_%d" % i, K.ellipse(x, y + s * 0.09, s * 0.22, s * 0.14 + 0.01 * (i % 2), 32), 0.0062, 0.007,
               parent=d, mat=K.flat(NIGHT if i % 2 else K.mix_hex(CORAL, NIGHT, 0.3)), line=False)


def build():
    ph = K.empty("Handy", loc=(0.45, 0.25, 0), rot=(0, 0, R(-10)))
    w, h = 0.66, 1.28
    d = yield from KR.big_phone("Handy", ph, w=w, h=h, body=NIGHT, screen=WHITE)
    K.shadow("Schatten_Handy", 0.45, 0.22, 0.42, 0.12)
    profile_screen(d, w - 0.06, h - 0.09)
    yield
    pe = K.empty("Person", loc=(-0.45, -0.1, 0), rot=(0, 0, R(22)))
    j = yield from KP.person("Person", pe,
                             dict(skin=SKIN[1], hair="quiff", hair_color=NIGHT, top=MINT, bottom=BLUE, shoes=WHITE,
                                  sleeves="long"),
                             dict(arm_r=((-34, -10, 0), -86), arm_l=((4, -8, 0), -8), head=(26, 0, 0),
                                  leg_r=((2, 0, 0), 0), leg_l=((-6, 0, 0), 4)))
    K.drop_to_floor(pe)
    hp = K.empty("Handy_klein_Platz", loc=(-0.05, -0.33, 0.33), rot=(R(-50), 0, R(175)), parent=j["pelvis"],
                 scale=(1.3, 1.3, 1.3))
    yield from KR.phone("Handy_klein", hp, check=False, screen=SKY_L)
    K.shadow("Schatten_Person", -0.45, -0.1, 0.3, 0.15)
    pl = K.empty("Pflanze", loc=(-1.25, 0.3, 0))
    yield from KR.plant("Pflanze", pl, pot=YELLOW, height=0.75, leaves=6)
    K.shadow("Schatten_Pflanze", -1.25, 0.3, 0.26, 0.13)
    yield


K.run("schritt-entdecken", build, (1200, 900), samples=24, frame_fill=(0.88, 0.88), cam=(-90, 8))
