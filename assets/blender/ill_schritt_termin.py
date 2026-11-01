# Schritt "im Stuhl": Kundin im Salonstuhl, Friseurin foehnt, Kalenderblatt mit Haken.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzpeople as KP
import kzprops as KR
from kzlib import SKIN, SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    # Kalender auf Staffelei links
    cb = K.empty("Kalender", loc=(-1.05, 0.15, 0), rot=(0, 0, R(14)))
    yield from KR.calendar_board("Kalender", cb, band_color=CORAL)
    K.shadow("Schatten_Kalender", -1.05, 0.2, 0.32, 0.13)
    # Stuhl mit Kundin
    ch = K.empty("Stuhl", loc=(0.05, 0.0, 0), rot=(0, 0, R(12)))
    yield from KR.salon_chair("Stuhl", ch, seat=YELLOW)
    K.shadow("Schatten_Stuhl", 0.05, 0.02, 0.5, 0.26)
    cl = K.empty("Kundin", loc=(0, 0.04, 0.57), parent=ch)
    jc = yield from KP.person("Kundin", cl,
                              dict(skin=SKIN[2], hair="bob", hair_color=K.P["haar_braun"], top=WHITE, bottom=NIGHT,
                                   shoes=MINT, cape=K.P["rosa"]),
                              dict(KP.SIT, head=(2, 0, 10)))
    # Friseurin rechts hinter dem Stuhl mit Foehn
    sy = K.empty("Friseurin", loc=(0.62, 0.28, 0.0), rot=(0, 0, R(-38)))
    j = yield from KP.person("Friseurin", sy,
                             dict(skin=SKIN[1], hair="ponytail", hair_color=K.P["haar_blond"], top=BLUE, bottom=NIGHT,
                                  shoes=WHITE, apron=NIGHT, apron_low=-0.2),
                             dict(arm_r=((-82, 10, 0), -40), arm_l=((-40, -18, 0), -70), head=(14, 0, 6),
                                  leg_r=((3, 0, 0), 0), leg_l=((-8, 0, 0), 6)))
    K.drop_to_floor(sy)
    import bpy
    bpy.context.view_layer.update()
    head = jc["head"].matrix_world.translation
    yield from KR.hold_dryer("Foehn", j["hand_r"], head, body=CORAL)
    K.shadow("Schatten_Friseurin", 0.62, 0.28, 0.3, 0.15)
    yield
    pl = K.empty("Pflanze", loc=(1.35, 0.35, 0))
    yield from KR.plant("Pflanze", pl, pot=WHITE, height=0.8, leaves=6)
    K.shadow("Schatten_Pflanze", 1.35, 0.35, 0.26, 0.13)
    yield


K.run("schritt-termin", build, (1200, 900), samples=24, frame_fill=(0.9, 0.9), cam=(-90, 9))
