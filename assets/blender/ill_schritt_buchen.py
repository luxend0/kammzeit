# Schritt "abends buchen": Kundin auf dem Sofa mit Handy, Mond im Fenster, Wecker zeigt 23:47.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    yield from KR.window_wall("Wand", K.empty("Wandstueck", loc=(0, 0.62, 0)), w=2.9, h=2.05, win_x=-0.45, win_y=1.5, win_w=0.9, win_h=0.9)
    # Teppich
    K.slab("Teppich", K.ellipse(0, 0, 1.25, 0.55, 96), 0.0, 0.003, mat=K.flat(K.mix_hex(CORAL, WHITE, 0.62)),
           line=False, loc=(0.05, -0.12, 0.0))
    so = K.empty("Sofa", loc=(-0.05, 0.12, 0))
    yield from KR.sofa("Sofa", so, color=BLUE, width=1.85)
    K.shadow("Schatten_Sofa", -0.05, 0.12, 1.0, 0.36)
    yield from KR.pillow("Kissen", K.empty("Kissen_Platz", loc=(-0.62, 0.2, 0.74), rot=(R(-14), R(12), R(8))), color=YELLOW)
    # Kundin, leicht zur Seite gedreht, Handy in beiden Haenden
    cl = K.empty("Kundin", loc=(0.38, 0.06, 0.64), rot=(0, 0, R(18)))
    j = yield from KP.person("Kundin", cl,
                             dict(skin=SKIN[4], hair="afro", hair_color=NIGHT, top=CORAL, bottom=SKY, shoes=WHITE,
                                  sleeves="long"),
                             dict(leg_r=((-84, 0, -2), 82), leg_l=((-88, 0, 6), 86),
                                  arm_r=((-15, -14, 0), -80), arm_l=((-15, 14, 0), -82),
                                  torso=(-6, 0, 0), head=(28, 0, 0)))
    ph = K.empty("Handy_Platz", loc=(0.0, -0.33, 0.29), rot=(R(-55), 0, R(165)), parent=j["pelvis"], scale=(1.35, 1.35, 1.35))
    yield from KR.phone("Handy", ph, check=False, screen=SKY_L)
    # Beistelltisch mit Wecker 23:47
    tb = K.empty("Tisch", loc=(1.28, -0.05, 0))
    yield from KR.side_table("Tisch", tb, color=WHITE)
    K.shadow("Schatten_Tisch", 1.28, -0.05, 0.24, 0.12)
    wk = K.empty("Wecker", loc=(1.26, -0.08, 0.56), rot=(0, 0, R(-16)))
    yield from KR.digital_clock("Wecker", wk, "23:47")
    # Stehlampe links
    la = K.empty("Lampe", loc=(-1.3, 0.25, 0))
    yield from KR.floor_lamp("Lampe", la, shade=YELLOW)
    K.shadow("Schatten_Lampe", -1.3, 0.25, 0.2, 0.1)
    yield


K.run("schritt-buchen", build, (1200, 900), samples=24, frame_fill=(0.9, 0.9), cam=(-90, 8))
