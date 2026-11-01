# Hero: Friseurin schneidet Kundin im Stuhl, Spiegelplatz mit Foehn, Pflanze, vorne rechts Kammi mit Handy.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    # Rueckwand mit grossem Fenster (Tag)
    yield from KR.window_wall("Wand", K.empty("Wandstueck", loc=(-0.25, 1.0, 0)), w=4.7, h=2.45, win_x=0.75,
                              win_y=1.55, win_w=1.35, win_h=1.25, day=True)
    yield from KR.wall_clock("Wanduhr", KR.upright("Wanduhr_Platz", None, loc=(-0.38, 0.94, 1.9)), r=0.19)
    yield from KR.wall_shelf("Regal", K.empty("Regal_Platz", loc=(1.78, 0.94, 1.42)))
    # Spiegelplatz links hinten
    st = K.empty("Spiegelplatz", loc=(-1.05, 0.55, 0), rot=(0, 0, R(6)))
    yield from KR.mirror_station("Platz", st, frame=YELLOW)
    K.shadow("Schatten_Platz", -1.05, 0.5, 0.62, 0.26)
    fo = K.empty("Foehn", loc=(-0.78, 0.36, 0.89), rot=(R(-90), 0, R(200)))
    yield from KR.dryer("Foehn", fo)
    # Pflanze ganz links
    pl = K.empty("Pflanze", loc=(-2.0, 0.3, 0))
    yield from KR.plant("Pflanze", pl, pot=CORAL, height=1.05)
    K.shadow("Schatten_Pflanze", -2.0, 0.3, 0.3, 0.16)
    # Stuhl mit Kundin
    ch = K.empty("Stuhl", loc=(0.0, 0.0, 0), rot=(0, 0, R(-14)))
    yield from KR.salon_chair("Stuhl", ch, seat=BLUE)
    K.shadow("Schatten_Stuhl", 0.0, 0.02, 0.5, 0.26)
    cl = K.empty("Kundin", loc=(0, 0.04, 0.57), parent=ch)
    yield from KP.person("Kundin", cl,
                         dict(skin=SKIN[0], hair="long", hair_color=CORAL, top=WHITE, bottom=NIGHT, shoes=CORAL,
                              cape=SKY),
                         dict(KP.SIT, head=(4, 0, -8)))
    # Friseurin hinter dem Stuhl, Schere und Kamm am Kopf der Kundin
    sy = K.empty("Friseurin", loc=(0.44, 0.3, 0.0), rot=(0, 0, R(-52)))
    j = yield from KP.person("Friseurin", sy,
                             dict(skin=SKIN[3], hair="bun", hair_color=NIGHT, top=MINT, bottom=BLUE, shoes=WHITE,
                                  apron=NIGHT, apron_low=-0.2),
                             dict(arm_r=((-96, 4, 6), -18), arm_l=((-35, -5, 0), -95), head=(16, 0, 4),
                                  leg_r=((3, 0, 0), 0), leg_l=((-8, 0, 0), 6)))
    yield from KR.scissors("Schere", j["hand_r"])
    yield from KR.comb("Kamm", j["hand_l"])
    K.drop_to_floor(sy)
    K.shadow("Schatten_Friseurin", 0.44, 0.3, 0.3, 0.15)
    yield
    # Kammi vorne rechts mit Handy (gruener Haken)
    km = K.kammi("Kammi", loc=(1.2, -0.6, 0), rot_z=-20, pose="handy", scale=1.45)
    K.shadow("Schatten_Kammi", 1.2, -0.6, 0.38, 0.17)
    yield
    hold = K.empty("Kammi_Handy_Halter")
    yield from KR.phone("Kammi_Handy", hold, check=True)
    K.kammi_hold(km, hold, "R", offset=(0, -0.04, 0.1), rot=(0, 0, -12), scale=1.6)
    yield


K.run("hero", build, (2000, 1500), samples=24, frame_fill=(0.94, 0.9), cam=(-90, 9))
