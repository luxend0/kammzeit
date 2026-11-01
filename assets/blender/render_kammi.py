# Kammi-Posen (800x800, transparent): KAMMI_POSE=winkt|handy|schere|kalender|schlaeft
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bpy
from mathutils import Vector
import toon_lib as T

POSE = os.environ.get("KAMMI_POSE", "winkt")
OUT = os.path.join(T.ASSETS, "ill", "png", "kammi-%s.png" % POSE)
CAM = {"winkt": (-74, 9), "handy": (-70, 9), "schere": (-76, 9), "kalender": (-80, 8), "schlaeft": (-66, 12)}


def build():
    T.set_light(-130, 48)
    k = T.append_kammi(pose=POSE)
    T.header("Kammi: Pose %s" % POSE)
    yield 0.8
    if POSE == "handy":
        ph = T.prop_phone("Handy")
        T.attach(ph, k, "R", offset=(0.01, -0.03, -0.01), world_rot=(4, 0, -8))
        yield
    elif POSE == "schere":
        sc = T.prop_scissors("Schere", open_deg=34)
        sc.scale = (1.45, 1.45, 1.45)
        T.attach(sc, k, "R", offset=(0.0, -0.04, 0.0), world_rot=(0, -18, 0))
        sc.scale = (1.45, 1.45, 1.45)
        yield
    elif POSE == "kalender":
        cal = T.prop_calendar("Kalender")
        T.attach(cal, k, "R", offset=(-0.075, -0.035, 0.045), world_rot=(0, 0, 8))
        yield
    elif POSE == "schlaeft":
        pts = T.world_points(T.descendants(k))
        top = max(p.z for p in pts)
        right = max(p.x for p in pts)
        front = min(p.y for p in pts)
        right += 0.1
        for i, (dx, dz, sz) in enumerate(((0.0, 0.07, 0.08), (0.09, 0.145, 0.066), (0.17, 0.21, 0.054))):
            T.text_mesh("Zzz_%d" % i, "Z", sz, color="blau", steps=2, depth=0.014,
                        loc=(right + dx, front - 0.05, top + dz), rot=(90, 0, -14 + i * 7))
            yield 0.4
    T.drop_to_floor(k)
    T.shadow([k], name="Schatten_Kammi", zmax=0.07, pad=1.2)
    yield


T.Run("kammi-" + POSE, build, out=OUT, res=(800, 800), samples=32, outline_px=2.6, frame_fill=0.80,
      cam=CAM.get(POSE, (-74, 9))).start()
